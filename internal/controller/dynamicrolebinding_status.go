package controller

import (
	"prosimcorp.com/kuberbac/internal/globals"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kuberbacv1alpha1 "prosimcorp.com/kuberbac/api/v1alpha1"
)

func (r *DynamicRoleBindingReconciler) UpdateConditionSuccess(resource *kuberbacv1alpha1.DynamicRoleBinding) {

	//
	condition := globals.NewCondition(globals.ConditionTypeResourceSynced, metav1.ConditionTrue,
		globals.ConditionReasonTargetSynced, globals.ConditionReasonTargetSyncedMessage)

	globals.UpdateCondition(&resource.Status.Conditions, condition)
}

func (r *DynamicRoleBindingReconciler) UpdateConditionKubernetesApiCallFailure(resource *kuberbacv1alpha1.DynamicRoleBinding) {

	//
	condition := globals.NewCondition(globals.ConditionTypeResourceSynced, metav1.ConditionTrue,
		globals.ConditionReasonKubernetesApiCallErrorType, globals.ConditionReasonKubernetesApiCallErrorMessage)

	globals.UpdateCondition(&resource.Status.Conditions, condition)
}
