using '../templates/mgmt-cluster.bicep'

param kubernetesVersion = '1.30.4'
param vnetAddressPrefix = '10.132.0.0/14'
param subnetPrefix = '10.132.8.0/21'
param podSubnetPrefix = '10.132.64.0/18'
param aksClusterName = 'aro-hcp-mgmt-cluster'
param aksKeyVaultName = take('aks-kv-mgmt-cluster-${uniqueString(currentUserId)}', 24)
param aksEtcdKVEnableSoftDelete = false
param systemAgentMinCount = 2
param systemAgentMaxCount = 3
param systemAgentVMSize = 'Standard_D2s_v3'
param aksSystemOsDiskSizeGB = 32
param userAgentMinCount = 1
param userAgentMaxCount = 6
param userAgentVMSize = 'Standard_D4s_v3'
param aksUserOsDiskSizeGB = 100
param userAgentPoolAZCount = 3
param persist = false

param deployMaestroConsumer = true
param maestroKeyVaultName = take('maestro-kv-${uniqueString(currentUserId)}', 24)
param maestroEventGridNamespacesName = take('maestro-eg-${uniqueString(currentUserId)}', 24)
param maestroCertDomain = 'selfsigned.maestro.keyvault.aro-int.azure.com'

param baseDNSZoneName = 'hcp.osadev.cloud'

param acrPullResourceGroups = ['global']

// These parameters are always overriden in the Makefile
param currentUserId = ''
param regionalResourceGroup = ''
