package main

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

func main() {
	config, err := rest.InClusterConfig()
	if err != nil || config == nil {
		fmt.Fprintf(os.Stderr, "Error: not running in a kubernetes cluster")
		// use local kubeconfig for debugging
		kubeconfig := os.Getenv("KUBECONFIG")
		config, err = clientcmd.BuildConfigFromFlags("", kubeconfig)
	}
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to load kubeconfig")
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
	}

	taint := corev1.Taint{
		Key:    "canary",
		Effect: corev1.TaintEffectNoSchedule,
	}

	serviceAccount := os.Getenv("CONTROLLER_SERVICE_ACCOUNT")

	for {
		// Find all nodes with the label kubernetes.azure.com/os-sku=Windows2022 and do not have the label winops/canary-status=complete
		nodes, err := clientset.CoreV1().Nodes().List(context.Background(), metav1.ListOptions{LabelSelector: "kubernetes.azure.com/os=Windows"})
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: failed to list nodes")
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		}

		for _, node := range nodes.Items {
			_, found := node.Labels["winops/canary-status"]
			if !found {
				//taint and label the node
				node.Spec.Taints = append(node.Spec.Taints, taint)
				node.Labels["winops/canary-status"] = "inprogress"
				_, err := clientset.CoreV1().Nodes().Update(context.Background(), &node, metav1.UpdateOptions{})
				if err != nil {
					fmt.Fprintf(os.Stderr, "Error: failed to update node %s\n", node.Name)
					fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				}

				//generate a unique naming suffix for this node
				suffix := getSuffix()

				//create a pod to disable av signature updates on the node
				p1 := createAVPod(suffix, node, clientset)
				fmt.Fprintf(os.Stdout, "Pod created: %s\n", p1.Name)

				//create a pod to remove the taint from the node once the AV update is complete and label the node complete
				p2 := createTaintCleanupPod(suffix, serviceAccount, node, clientset)
				fmt.Fprintf(os.Stdout, "Pod created: %s\n", p2.Name)
			}
		}

		time.Sleep(60 * time.Second)
	}
}

// Generate a random string of length 6
func getSuffix() string {
	chars := []rune("abcdefghijklmnopqrstuvwxyz0123456789")
	length := 6
	var b []rune
	b = append(b, '-')
	for i := 0; i < length; i++ {
		b = append(b, chars[rand.Intn(len(chars))])
	}
	return string(b)
}

// createPod creates a pod to disable av signature updates on the node
func createAVPod(suffix string, node corev1.Node, c kubernetes.Interface) *corev1.Pod {
	hostProcess := true
	runAsUserName := "NT AUTHORITY\\SYSTEM"

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "disable-av-signature-updates" + suffix,
			Namespace: "default",
			Labels: map[string]string{
				"app": "disable-av-signature-updates",
			},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:    "disable-av-signature-updates",
					Image:   "mcr.microsoft.com/oss/kubernetes/windows-host-process-containers-base-image:v1.0.0",
					Command: []string{"powershell"},
					Args: []string{"reg add 'HKEY_LOCAL_MACHINE\\SOFTWARE\\Microsoft\\Windows Defender\\Signature Updates' /v FallbackOrder /t REG_SZ /d 'FileShares' /f;",
						"reg add 'HKEY_LOCAL_MACHINE\\SOFTWARE\\Microsoft\\Windows Defender\\Signature Updates' /v SignatureUpdateInterval /t REG_DWORD /d 0 /f;"},
					SecurityContext: &corev1.SecurityContext{
						WindowsOptions: &corev1.WindowsSecurityContextOptions{
							HostProcess:   &hostProcess,
							RunAsUserName: &runAsUserName,
						},
					},
				},
			},
			Tolerations: []corev1.Toleration{
				{
					Key:      "canary",
					Operator: corev1.TolerationOpExists,
					Effect:   corev1.TaintEffectNoSchedule,
				},
			},
			NodeSelector: map[string]string{
				"kubernetes.io/hostname": node.Name,
			},
			RestartPolicy: corev1.RestartPolicyOnFailure,
		},
	}
	if _, err := c.CoreV1().Pods("default").Create(context.Background(), pod, metav1.CreateOptions{}); err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to create pod %s\n", pod.Name)
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
	}

	return pod
}

func createTaintCleanupPod(suffix string, serviceAcount string, node corev1.Node, c kubernetes.Interface) *corev1.Pod {
	image := os.Getenv("CONTROLLER_IMAGE")

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "remove-canary-taint-" + suffix,
			Namespace: "default",
			Labels: map[string]string{
				"app": "remove-canary-taint",
			},
		},
		Spec: corev1.PodSpec{
			ServiceAccountName: serviceAcount,
			Containers: []corev1.Container{
				{
					Name:  "remove-canary-taint",
					Image: image,
					Env: []corev1.EnvVar{
						{
							Name:  "NODE_NAME",
							Value: node.Name,
						},
					},
				},
			},
			Tolerations: []corev1.Toleration{
				{
					Key:      "canary",
					Operator: corev1.TolerationOpExists,
					Effect:   corev1.TaintEffectNoSchedule,
				},
			},
			NodeSelector: map[string]string{
				"kubernetes.io/hostname": node.Name,
			},
			RestartPolicy: corev1.RestartPolicyOnFailure,
		},
	}
	if _, err := c.CoreV1().Pods("default").Create(context.Background(), pod, metav1.CreateOptions{}); err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to create pod %s\n", pod.Name)
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
	}

	return pod
}
