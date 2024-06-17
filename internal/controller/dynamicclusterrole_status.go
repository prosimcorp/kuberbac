package controller

import (
	"prosimcorp.com/kuberbac/internal/globals"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kuberbacv1alpha1 "prosimcorp.com/kuberbac/api/v1alpha1"
)

func (r *DynamicClusterRoleReconciler) UpdateConditionSuccess(dynamicClusterRole *kuberbacv1alpha1.DynamicClusterRole) {

	//
	condition := globals.NewCondition(globals.ConditionTypeResourceSynced, metav1.ConditionTrue,
		globals.ConditionReasonTargetSynced, globals.ConditionReasonTargetSyncedMessage)

	globals.UpdateCondition(&dynamicClusterRole.Status.Conditions, condition)
}

func (r *DynamicClusterRoleReconciler) UpdateConditionKubernetesApiCallFailure(dynamicClusterRole *kuberbacv1alpha1.DynamicClusterRole) {

	//
	condition := globals.NewCondition(globals.ConditionTypeResourceSynced, metav1.ConditionTrue,
		globals.ConditionReasonKubernetesApiCallErrorType, globals.ConditionReasonKubernetesApiCallErrorMessage)

	globals.UpdateCondition(&dynamicClusterRole.Status.Conditions, condition)
}
