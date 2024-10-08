package frontend

import (
	"context"
	"fmt"

	cmv1 "github.com/openshift-online/ocm-sdk-go/clustersmgmt/v1"
	configv1 "github.com/openshift/api/config/v1"

	"github.com/Azure/ARO-HCP/internal/api"
	"github.com/Azure/ARO-HCP/internal/api/arm"
)

const (
	csFlavourId        string = "osd-4" // managed cluster
	csCloudProvider    string = "azure"
	csProductId        string = "aro"
	csHypershifEnabled bool   = true
	csMultiAzEnabled   bool   = true
	csCCSEnabled       bool   = true
)

func convertListeningToVisibility(listening cmv1.ListeningMethod) (visibility api.Visibility) {
	switch listening {
	case cmv1.ListeningMethodExternal:
		visibility = api.VisibilityPublic
	case cmv1.ListeningMethodInternal:
		visibility = api.VisibilityPrivate
	}
	return
}

func convertVisibilityToListening(visibility api.Visibility) (listening cmv1.ListeningMethod) {
	switch visibility {
	case api.VisibilityPublic:
		listening = cmv1.ListeningMethodExternal
	case api.VisibilityPrivate:
		listening = cmv1.ListeningMethodInternal
	}
	return
}

// ConvertCStoHCPOpenShiftCluster converts a CS Cluster object into HCPOpenShiftCluster object
func ConvertCStoHCPOpenShiftCluster(resourceID *arm.ResourceID, cluster *cmv1.Cluster) *api.HCPOpenShiftCluster {
	hcpcluster := &api.HCPOpenShiftCluster{
		TrackedResource: arm.TrackedResource{
			Location: cluster.Region().ID(),
			Resource: arm.Resource{
				ID:   resourceID.String(),
				Name: resourceID.Name,
				Type: resourceID.ResourceType.String(),
			},
		},
		Properties: api.HCPOpenShiftClusterProperties{
			// ProvisioningState: cluster.State(), // TODO: align with OCM on ProvisioningState
			Spec: api.ClusterSpec{
				Version: api.VersionProfile{
					ID:                cluster.Version().ID(),
					ChannelGroup:      cluster.Version().ChannelGroup(),
					AvailableUpgrades: cluster.Version().AvailableUpgrades(),
				},
				DNS: api.DNSProfile{
					BaseDomain:       cluster.DNS().BaseDomain(),
					BaseDomainPrefix: cluster.DomainPrefix(),
				},
				Network: api.NetworkProfile{
					NetworkType: api.NetworkType(cluster.Network().Type()),
					PodCIDR:     cluster.Network().PodCIDR(),
					ServiceCIDR: cluster.Network().ServiceCIDR(),
					MachineCIDR: cluster.Network().MachineCIDR(),
					HostPrefix:  int32(cluster.Network().HostPrefix()),
				},
				Console: api.ConsoleProfile{
					URL: cluster.Console().URL(),
				},
				API: api.APIProfile{
					URL:        cluster.API().URL(),
					Visibility: convertListeningToVisibility(cluster.API().Listening()),
				},
				FIPS:                          cluster.FIPS(),
				EtcdEncryption:                cluster.EtcdEncryption(),
				DisableUserWorkloadMonitoring: cluster.DisableUserWorkloadMonitoring(),
				Proxy: api.ProxyProfile{
					HTTPProxy:  cluster.Proxy().HTTPProxy(),
					HTTPSProxy: cluster.Proxy().HTTPSProxy(),
					NoProxy:    cluster.Proxy().NoProxy(),
					TrustedCA:  cluster.AdditionalTrustBundle(),
				},
				Platform: api.PlatformProfile{
					ManagedResourceGroup:   cluster.Azure().ManagedResourceGroupName(),
					SubnetID:               cluster.Azure().SubnetResourceID(),
					OutboundType:           api.OutboundTypeLoadBalancer,
					NetworkSecurityGroupID: cluster.Azure().NetworkSecurityGroupResourceID(),
					EtcdEncryptionSetID:    "",
				},
				IssuerURL: "",
				ExternalAuth: api.ExternalAuthConfigProfile{
					Enabled:       false,
					ExternalAuths: []*configv1.OIDCProvider{},
				},
			},
		},
	}

	return hcpcluster
}

// BuildCSCluster creates a CS Cluster object from an HCPOpenShiftCluster object
func (f *Frontend) BuildCSCluster(ctx context.Context, hcpCluster *api.HCPOpenShiftCluster, updating bool) (*cmv1.Cluster, error) {
	resourceID, err := ResourceIDFromContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("could not get parsed resource ID: %w", err)
	}
	tenantID, err := TenantIDFromContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("could not get tenant ID: %w", err)
	}

	// additionalProperties should be empty in production, it is configurable for development to pin to specific
	// provision shards or instruct CS to skip the full provisioning/deprovisioning flow.
	additionalProperties := map[string]string{
		// Enable the ARO HCP provisioner during development. For now, if not set a cluster will not progress past the
		// installing state in CS.
		"provisioner_hostedcluster_step_enabled": "true",
		// Enable the provisioning of ACM's ManagedCluster CR associated to the ARO-HCP
		// cluster during ARO-HCP Cluster provisioning. For now, if not set a cluster will not progress past the
		// installing state in CS.
		"provisioner_managedcluster_step_enabled": "true",

		// Enable the provisioning and deprovisioning of ARO-HCP Node Pools. For now, if not set the provisioning
		// and deprovisioning of day 2 ARO-HCP Node Pools will not be performed on the Management Cluster.
		"np_provisioner_provision_enabled":   "true",
		"np_provisioner_deprovision_enabled": "true",
	}
	if f.clusterServiceConfig.ProvisionShardID != nil {
		additionalProperties["provision_shard_id"] = *f.clusterServiceConfig.ProvisionShardID
	}
	if f.clusterServiceConfig.ProvisionerNoOpProvision {
		additionalProperties["provisioner_noop_provision"] = "true"
	}
	if f.clusterServiceConfig.ProvisionerNoOpDeprovision {
		additionalProperties["provisioner_noop_deprovision"] = "true"
	}

	clusterBuilder := cmv1.NewCluster()

	// FIXME HcpOpenShiftCluster attributes not being passed:
	//       PlatformProfile.OutboundType        (no CS equivalent?)
	//       PlatformProfile.EtcdEncryptionSetID (no CS equivalent?)
	//       ExternalAuth                        (TODO, complicated)

	// These attributes cannot be updated after cluster creation.
	if !updating {
		clusterBuilder = clusterBuilder.
			Name(hcpCluster.Name).
			Flavour(cmv1.NewFlavour().
				ID(csFlavourId)).
			Region(cmv1.NewCloudRegion().
				ID(f.location)).
			CloudProvider(cmv1.NewCloudProvider().
				ID(csCloudProvider)).
			Azure(cmv1.NewAzure().
				TenantID(tenantID).
				SubscriptionID(resourceID.SubscriptionID).
				ResourceGroupName(resourceID.ResourceGroupName).
				ResourceName(hcpCluster.Name).
				ManagedResourceGroupName(hcpCluster.Properties.Spec.Platform.ManagedResourceGroup).
				SubnetResourceID(hcpCluster.Properties.Spec.Platform.SubnetID).
				NetworkSecurityGroupResourceID(hcpCluster.Properties.Spec.Platform.NetworkSecurityGroupID)).
			Product(cmv1.NewProduct().
				ID(csProductId)).
			Hypershift(cmv1.NewHypershift().
				Enabled(csHypershifEnabled)).
			MultiAZ(csMultiAzEnabled).
			CCS(cmv1.NewCCS().Enabled(csCCSEnabled)).
			Version(cmv1.NewVersion().
				ID(hcpCluster.Properties.Spec.Version.ID).
				ChannelGroup(hcpCluster.Properties.Spec.Version.ChannelGroup)).
			Network(cmv1.NewNetwork().
				Type(string(hcpCluster.Properties.Spec.Network.NetworkType)).
				PodCIDR(hcpCluster.Properties.Spec.Network.PodCIDR).
				ServiceCIDR(hcpCluster.Properties.Spec.Network.ServiceCIDR).
				MachineCIDR(hcpCluster.Properties.Spec.Network.MachineCIDR).
				HostPrefix(int(hcpCluster.Properties.Spec.Network.HostPrefix))).
			API(cmv1.NewClusterAPI().
				Listening(convertVisibilityToListening(hcpCluster.Properties.Spec.API.Visibility))).
			FIPS(hcpCluster.Properties.Spec.FIPS).
			EtcdEncryption(hcpCluster.Properties.Spec.EtcdEncryption)

		// Cluster Service rejects an empty DomainPrefix string.
		if hcpCluster.Properties.Spec.DNS.BaseDomainPrefix != "" {
			clusterBuilder = clusterBuilder.
				DomainPrefix(hcpCluster.Properties.Spec.DNS.BaseDomainPrefix)
		}
	}

	proxyBuilder := cmv1.NewProxy()
	// Cluster Service allows an empty HTTPProxy on PATCH but not PUT.
	if updating || hcpCluster.Properties.Spec.Proxy.HTTPProxy != "" {
		proxyBuilder = proxyBuilder.
			HTTPProxy(hcpCluster.Properties.Spec.Proxy.HTTPProxy)
	}
	// Cluster Service allows an empty HTTPSProxy on PATCH but not PUT.
	if updating || hcpCluster.Properties.Spec.Proxy.HTTPSProxy != "" {
		proxyBuilder = proxyBuilder.
			HTTPSProxy(hcpCluster.Properties.Spec.Proxy.HTTPSProxy)
	}
	// Cluster Service allows an empty HTTPSProxy on PATCH but not PUT.
	if updating || hcpCluster.Properties.Spec.Proxy.NoProxy != "" {
		proxyBuilder = proxyBuilder.
			NoProxy(hcpCluster.Properties.Spec.Proxy.NoProxy)
	}

	clusterBuilder = clusterBuilder.
		DisableUserWorkloadMonitoring(hcpCluster.Properties.Spec.DisableUserWorkloadMonitoring).
		Proxy(proxyBuilder).
		AdditionalTrustBundle(hcpCluster.Properties.Spec.Proxy.TrustedCA).
		Properties(additionalProperties)

	cluster, err := clusterBuilder.Build()
	if err != nil {
		return nil, err
	}
	return cluster, nil
}

// ConvertCStoNodePool converts a CS Node Pool object into HCPOpenShiftClusterNodePool object
func ConvertCStoNodePool(resourceID *arm.ResourceID, np *cmv1.NodePool) *api.HCPOpenShiftClusterNodePool {
	nodePool := &api.HCPOpenShiftClusterNodePool{
		TrackedResource: arm.TrackedResource{
			Resource: arm.Resource{
				ID:   resourceID.String(),
				Name: resourceID.Name,
				Type: resourceID.ResourceType.String(),
			},
		},
		Properties: api.HCPOpenShiftClusterNodePoolProperties{
			// ProvisioningState: np.Status(), // TODO: Align with OCM on aligning with ProvisioningState
			Spec: api.NodePoolSpec{
				Version: api.VersionProfile{
					ID:                np.Version().ID(),
					ChannelGroup:      np.Version().ChannelGroup(),
					AvailableUpgrades: np.Version().AvailableUpgrades(),
				},
				Platform: api.NodePoolPlatformProfile{
					SubnetID:               np.Subnet(),
					VMSize:                 np.AzureNodePool().VMSize(),
					DiskStorageAccountType: np.AzureNodePool().OSDiskStorageAccountType(),
					AvailabilityZone:       np.AvailabilityZone(),
					EncryptionAtHost:       false, // TODO: Not implemented in OCM
					DiskSizeGiB:            int32(np.AzureNodePool().OSDiskSizeGibibytes()),
					DiskEncryptionSetID:    "", // TODO: Not implemented in OCM
					EphemeralOSDisk:        np.AzureNodePool().EphemeralOSDiskEnabled(),
				},
				Replicas:   int32(np.Replicas()),
				AutoRepair: np.AutoRepair(),
				Autoscaling: api.NodePoolAutoscaling{
					Min: int32(np.Autoscaling().MinReplica()),
					Max: int32(np.Autoscaling().MaxReplica()),
				},
				Labels:        np.Labels(),
				TuningConfigs: np.TuningConfigs(),
			},
		},
	}

	taints := make([]*api.Taint, len(np.Taints()))
	for i, t := range np.Taints() {
		taints[i] = &api.Taint{
			Effect: api.Effect(t.Effect()),
			Key:    t.Key(),
			Value:  t.Value(),
		}
	}
	nodePool.Properties.Spec.Taints = taints

	return nodePool
}

// BuildCSNodePool creates a CS Node Pool object from an HCPOpenShiftClusterNodePool object
func (f *Frontend) BuildCSNodePool(ctx context.Context, nodePool *api.HCPOpenShiftClusterNodePool, updating bool) (*cmv1.NodePool, error) {
	npBuilder := cmv1.NewNodePool()

	// FIXME HCPOpenShiftClusterNodePool attributes not being passed:
	//       PlatformProfile.EncryptionAtHost    (no CS equivalent?)
	//       PlatformProfile.DiskEncryptionSetID (no CS equivalent?)

	// These attributes cannot be updated after node pool creation.
	if !updating {
		npBuilder = npBuilder.
			ID(nodePool.Name).
			Version(cmv1.NewVersion().
				ID(nodePool.Properties.Spec.Version.ID).
				ChannelGroup(nodePool.Properties.Spec.Version.ChannelGroup).
				AvailableUpgrades(nodePool.Properties.Spec.Version.AvailableUpgrades...)).
			Subnet(nodePool.Properties.Spec.Platform.SubnetID).
			AzureNodePool(cmv1.NewAzureNodePool().
				ResourceName(nodePool.Name).
				VMSize(nodePool.Properties.Spec.Platform.VMSize).
				OSDiskSizeGibibytes(int(nodePool.Properties.Spec.Platform.DiskSizeGiB)).
				OSDiskStorageAccountType(nodePool.Properties.Spec.Platform.DiskStorageAccountType).
				EphemeralOSDiskEnabled(nodePool.Properties.Spec.Platform.EphemeralOSDisk)).
			AvailabilityZone(nodePool.Properties.Spec.Platform.AvailabilityZone).
			AutoRepair(nodePool.Properties.Spec.AutoRepair)
	}

	npBuilder = npBuilder.
		Labels(nodePool.Properties.Spec.Labels).
		TuningConfigs(nodePool.Properties.Spec.TuningConfigs...)

	// from CS API: "Only one of 'replicas' and 'autoscaling' can be provided.
	if nodePool.Properties.Spec.Replicas != 0 {
		npBuilder.Replicas(int(nodePool.Properties.Spec.Replicas))
	} else {
		npBuilder.Autoscaling(cmv1.NewNodePoolAutoscaling().
			MinReplica(int(nodePool.Properties.Spec.Autoscaling.Min)).
			MaxReplica(int(nodePool.Properties.Spec.Autoscaling.Max)))
	}

	for _, t := range nodePool.Properties.Spec.Taints {
		npBuilder = npBuilder.Taints(cmv1.NewTaint().
			Effect(string(t.Effect)).
			Key(t.Key).
			Value(t.Value))
	}

	return npBuilder.Build()
}
