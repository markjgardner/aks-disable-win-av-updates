// this app will be running in a kubernetes pod and will monitor the status of a running pod. When that pod is marked complete this pod will remove the canary taint and label the node with winops/canary-status=complete.

package main

import (
	"context"
	"fmt"
	"os"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
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
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "Error: not running in a kubernetes cluster")
	}
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "Error: failed to load kubeconfig")
	}

	//Get the current nodename
	nodeName := os.Getenv("NODE_NAME")
	
	// get all pods with the label app=disable-av-signature-updates on the target node
	pods, _ := clientset.CoreV1().Pods("default").List(context.Background(), metav1.ListOptions{LabelSelector: "app=disable-av-signature-updates", FieldSelector: "spec.nodeName=" + nodeName })
	fmt.Fprintf(os.Stdout, "Found %d pods on host %s\n", len(pods.Items), nodeName)
	
	if len(pods.Items) == 1 {
		pod := &pods.Items[0]

		waiting := true
		// wait for the pod to complete
		for waiting {
			if pod.Status.Phase == "Succeeded" {
				// remove the taint and label from the node
				node, _ := clientset.CoreV1().Nodes().Get(context.Background(), nodeName, metav1.GetOptions{})
				node.Spec.Taints = removeTaint(node.Spec.Taints, "canary")
				node.Labels["winops/canary-status"] = "complete"
				clientset.CoreV1().Nodes().Update(context.Background(), node, metav1.UpdateOptions{})
				waiting = false
			} else {
				pod, _ = clientset.CoreV1().Pods("default").Get(context.Background(), pod.Name, metav1.GetOptions{})
			}
		}
	} else {
		_, _ = fmt.Fprintf(os.Stderr, "Error: more than one canary pod found")
	}
}
