// this app will be running in a kubernetes pod and will monitor the status of a running pod. When that pod is marked complete this pod will remove the canary taint and label the node with winops/canary/status=complete.

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
	nodeName, err := os.Hostname()
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "Error: could not read node hostname")
	}

	waiting := true
	// get all pods with the name "canary"
	for waiting {
		pods, _ := clientset.CoreV1().Pods("default").List(context.Background(), metav1.ListOptions{LabelSelector: "winops/canary/status=inprogress", FieldSelector: "spec.nodeName=" + nodeName})

		if pods == nil {
			_, _ = fmt.Fprintf(os.Stderr, "Error: no canary pods are running on this node")
		}

		// if there is more than one pod in the list throw an error
		if len(pods.Items) > 1 {
			_, _ = fmt.Fprintf(os.Stderr, "Error: more than one canary pod is running on this node")
		}

		pod := pods.Items[0]

		if pod.Status.Phase == "Succeeded" {
			// remove the taint and label from the node
			node, _ := clientset.CoreV1().Nodes().Get(context.Background(), nodeName, metav1.GetOptions{})
			node.Spec.Taints = removeTaint(node.Spec.Taints, "canary")
			node.Labels["winops/canary/status"] = "complete"
			clientset.CoreV1().Nodes().Update(context.Background(), node, metav1.UpdateOptions{})
			waiting = false
		}
	}
}
