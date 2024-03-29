package main

import (
	"context"
	"fmt"
	"os"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

func removeTaint(taints []corev1.Taint, taintToRemove string) []corev1.Taint {
	var newTaints []corev1.Taint
	for _, taint := range taints {
		if taint.Key != taintToRemove {
			newTaints = append(newTaints, taint)
		}
	}
	return newTaints
}

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
	}

	//Get the current nodename
	nodeName := os.Getenv("NODE_NAME")
	fmt.Fprintf(os.Stdout, "Node Name: %s\n", nodeName)

	// get all pods with the label app=disable-av-signature-updates on the target node
	poll := true
	for poll {
		pods, _ := clientset.CoreV1().Pods("default").List(context.Background(), metav1.ListOptions{LabelSelector: "app=disable-av-signature-updates", FieldSelector: "spec.nodeName=" + nodeName})
		if len(pods.Items) == 1 {
			poll = false
			pod := &pods.Items[0]
			waiting := true
			// wait for the pod to complete
			for waiting {
				if pod.Status.Phase == "Succeeded" {
					// remove the taint and add label
					node, _ := clientset.CoreV1().Nodes().Get(context.Background(), nodeName, metav1.GetOptions{})
					node.Spec.Taints = removeTaint(node.Spec.Taints, "canary")
					node.Labels["winops/canary-status"] = "complete"
					_, err := clientset.CoreV1().Nodes().Update(context.Background(), node, metav1.UpdateOptions{})
					if err != nil {
						_, _ = fmt.Fprintf(os.Stderr, "Error: failed to update node, retry in 10 seconds\n")
						time.Sleep(10 * time.Second)
					} else {
						fmt.Fprintf(os.Stdout, "Pod %s is complete, removed taint and added label\n", pod.Name)
						waiting = false
					}
				} else {
					fmt.Fprintf(os.Stdout, "Pod %s is not complete, waiting 10 seconds\n", pod.Name)
					time.Sleep(10 * time.Second)
					pod, _ = clientset.CoreV1().Pods("default").Get(context.Background(), pod.Name, metav1.GetOptions{})
				}
			}
		} else {
			_, _ = fmt.Fprintf(os.Stderr, "Error: pod count not exactly 1 -- %d\n", len(pods.Items))
			time.Sleep(10 * time.Second)
		}
	}

	os.Exit(0)
}
