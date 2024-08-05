package controller

const (
	DynamicClusterRoleResourceType = "DynamicClusterRole"
	DynamicRoleBindingResourceType = "DynamicRoleBinding"

	//
	scheduleSynchronization = "Schedule synchronization for %s '%s' in: %s"

	//
	resourceNotFoundError          = "%s '%s' resource not found. Ignoring since object must be deleted."
	resourceRetrievalError         = "Error getting the %s '%s' from the cluster: %s"
	resourceFinalizersUpdateError  = "Failed to update finalizer of %s '%s': %s"
	resourceConditionUpdateError   = "Failed to update the condition on %s '%s': %s"
	resourceSyncTimeRetrievalError = "Can not get synchronization time from the %s '%s': %s"
	syncTargetError                = "Can not sync the target for the %s '%s': %s"

	//
	patchFinalizer = "kuberbac.prosimcorp.com/finalizer"
)
