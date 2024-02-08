/*
Copyright 2024.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	nodev1alpha1 "winops/api/v1alpha1"
)

// CanaryReconciler reconciles a Canary object
type CanaryReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=node.winops,resources=canaries,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=node.winops,resources=canaries/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=node.winops,resources=canaries/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Canary object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.15.0/pkg/reconcile
func (r *CanaryReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	_ = log.FromContext(ctx)

	// Find all nodes with the label kubernetes.azure.com/os-sku=Windows2022 and do not have the label winops/canary/status=complete
	nodes := &corev1.NodeList{}
	err := r.Client.List(context.Background(), nodes, client.MatchingLabels{"kubernetes.azure.com/os-sku": "Windows2022"})
	if err != nil {
		// handle error
	}

	for _, node := range nodes.Items {
		value, found := node.Labels["winops/canary/status"]
		if !found {
			//taint and label the node
			taint := corev1.Taint{
				Key:    "canary",
				Effect: corev1.TaintEffectNoSchedule,
			}
			node.Spec.Taints = append(node.Spec.Taints, taint)
			label := map[string]string{"winops/canary/status": "inprogress"}
			node.Labels.Merge(label)
			r.Client.Update(context.Background(), &node)

			//generate a unique naming suffix for this node
			suffix := getSuffix()

			//create a pod to disable av signature updates on the node
			p1 := createAVPod(suffix, node, r)
			log.FromContext(ctx).Info("Pod created", "Pod.Name", p1.Name)

			//use the service account provided in the operator config
			serviceAccount := ctx.Value("ServiceAccountName").(string)

			//create a pod to remove the taint from the node once the AV update is complete and label the node complete
			p2 := createTaintCleanupPod(suffix, serviceAccount, node, r)
			log.FromContext(ctx).Info("Pod created", "Pod.Name", p2.Name)
		}
	}

	return ctrl.Result{}, nil
}

// Generate a random string of length 6
func getSuffix() string {
    rand.Seed(time.Now().UnixNano())
    chars := []rune("ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789")
    length := 6
    var b []rune
    for i := 0; i < length; i++ {
        b = append(b, chars[rand.Intn(len(chars))])
    }
    return string(b)
}

// createPod creates a pod to disable av signature updates on the node
func createAVPod(suffix string, node corev1.Node, r *CanaryReconciler) *corev1.Pod {
	hostProcess := true
	runAsUserName := "NT AUTHORITY\\SYSTEM"
	randomstring := 

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "disable-av-signature-updates",
			Namespace: "default",
		},
		Labels: map[string]string{
			"app": "disable-av-signature-updates",
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:    "disable-av-signature-updates-" + suffix,
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
				"kubernetes.azure.com/os-sku": "Windows2022",
				"kubernetes.io/hostname":      node.Name,
			},
		},
	}
	if err := r.Client.Create(context.Background(), pod); err != nil {
		//handle error
	}

	return pod
}

func createTaintCleanupPod(suffix string, serviceAcount string, node corev1.Node, r *CanaryReconciler) *corev1.Pod {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "remove-canary-taint-" + suffix,
			Namespace: "default",
		},
		Spec: corev1.PodSpec{
			ServiceAccountName: serviceAcount,
			Containers: []corev1.Container{
				{
					Name:  "remove-canary-taint",
					Image: "replace-with-your-image",
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
				"kubernetes.azure.com/os-sku": "Windows2022",
				"kubernetes.io/hostname":      node.Name,
			},
		},
	}
	if err := r.Client.Create(context.Background(), pod); err != nil {
		//handle error
	}

	return pod
}

// SetupWithManager sets up the controller with the Manager.
func (r *CanaryReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&nodev1alpha1.Canary{}).
		Complete(r)
}
