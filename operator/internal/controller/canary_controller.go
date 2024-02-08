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
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

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
	if err := r.Get(ctx, req.NamespacedName, &node); err != nil {
		// handle error
	}

	// Find all nodes with the label kubernetes.azure.com/os-sku=Windows2022 and do not have the label winops/canary/status=complete
	nodes := &corev1.NodeList{}
	err := r.Client.List(context.Background(), nodes, client.MatchingLabels{"kubernetes.azure.com/os-sku": "Windows2022"})
	if err != nil {
		// handle error
	}

	for _, node := range nodes.Items {
		value, found := node.Labels["winops/canary/status"]
		if !found || value != "complete" {
			//taint the node
			taint := corev1.Taint{
				Key:    "canary",
				Effect: corev1.TaintEffectNoSchedule,
			}
			node.Spec.Taints = append(node.Spec.Taints, taint)
			r.Client.Update(context.Background(), &node)

			//create a service account with the ability to remove taints from nodes
			serviceAcount := createServiceAccount(r)

			//create a pod to disable av signature updates on the node
			createAVPod(serviceAcount, node, r)

			//create a pod to remove the taint from the node once the AV update is complete
			createTaintCleanupPod(serviceAcount, node, r)
		}
	}

	return ctrl.Result{}, nil
}

// createPod creates a pod to disable av signature updates on the node
func createAVPod(serviceAcount *corev1.ServiceAccount, node corev1.Node, r *CanaryReconciler) *corev1.Pod {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "disable-av-signature-updates",
			Namespace: "default",
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
							HostProcess:   true,
							RunAsUserName: "NT AUTHORITY\\SYSTEM",
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

func createTainCleanupPod(serviceAcount *corev1.ServiceAccount, node corev1.Node, r *CanaryReconciler) *corev1.Pod {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "remove-canary-taint",
			Namespace: "default",
		},
		Spec: corev1.PodSpec{
			ServiceAccountName: serviceAcount.Name,
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

// createServiceAccount creates a service account with the ability to remove taints from nodes
func createServiceAccount(r *CanaryReconciler) *corev1.ServiceAccount {
	serviceAccount := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "canary-service-account",
			Namespace: "default",
		},
	}

	if err := r.Client.Create(context.Background(), serviceAccount); err != nil {
		// handle error
	}

	role := &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "canary-role",
			Namespace: "default",
		},
		Rules: []rbacv1.PolicyRule{
			{
				APIGroups: []string{""},
				Resources: []string{"nodes"},
				Verbs:     []string{"taint"},
			},
		},
	}

	if err := r.Client.Create(context.Background(), role); err != nil {
		// handle error
	}

	roleBinding := &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "canary-role-binding",
			Namespace: "default",
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      "ServiceAccount",
				Name:      serviceAccount.Name,
				Namespace: "default",
			},
		},
		RoleRef: rbacv1.RoleRef{
			Kind:     "Role",
			Name:     role.Name,
			APIGroup: "rbac.authorization.k8s.io",
		},
	}

	if err := r.Client.Create(context.Background(), roleBinding); err != nil {
		// handle error
	}

	return serviceAccount
}

// SetupWithManager sets up the controller with the Manager.
func (r *CanaryReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&nodev1alpha1.Canary{}).
		Complete(r)
}
