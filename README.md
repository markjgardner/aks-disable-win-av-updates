# aks-disable-win-av-updates
[Operator](/operator) contains logic for monitoring the cluster for new nodes which have not yet had the canary label applied to them indicating that they require action to be taken. This container requires an environment variable of ```CONTROLLER_SERVICE_ACCOUNT``` containing the serviceAccount name that the nodeController pod should use and ```CONTROLLER_IMAGE``` indicating what container image should be used for the controller. When the operator detects a new node it immediately taints and labels the node and then schedules two pods to run:

1. a host process container which sets two registry keys disabling signature updates for defender on the node
2. a cleanup pod that watches for the first pod to terminate and then removes the taint and sets the label value to 'complete' on the target node

[nodeController](/nodeController) contains the logic for the second pod in the list above. This contianer requires an environment variable of ```NODE_NAME``` containing the name of the node it is targeting. 

[DisableAvSignatureUpdates.yml](/DisableAvSignatureUpdates.yml) holds an example deployment of this workload with neccessary roles and bindings defined.
