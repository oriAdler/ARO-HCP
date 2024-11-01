SHELL = /bin/bash
COMMIT = $(shell git rev-parse --short=7 HEAD)$(shell [[ $$(git status --porcelain) = "" ]])
CONFIG_PROFILE ?= dev
include ../../dev-infrastructure/configurations/$(CONFIG_PROFILE).mk
NAMESPACE ?= image-sync

deploy-shared:
	AZURE_SYNC_MI_CLIENT_ID=$(shell az identity show \
			-g ${RESOURCEGROUP} \
			-n image-sync \
			--query clientId) && \
	helm upgrade --force --install shared-resources ./shared-resources \
	--namespace ${NAMESPACE} --create-namespace \
	--set serviceAccountName=image-sync \
	--set azureClientId=$${AZURE_SYNC_MI_CLIENT_ID}

deploy-oc-mirror:
	AZURE_SYNC_MI_CLIENT_ID=$(shell az identity show \
			-g ${RESOURCEGROUP} \
			-n image-sync \
			--query clientId) && \
	TENANT_ID=$(shell az account show --query tenantId --output tsv) && \
	helm upgrade --force --install oc-mirror-cronjob ./ocmirrorCronjob \
	--namespace ${NAMESPACE} --create-namespace \
	--set serviceAccountName=image-sync \
	--set azureClientId=$${AZURE_SYNC_MI_CLIENT_ID} \
	--set acrRegistryName=${ARO_HCP_IMAGE_ACR} \
	--set azureTenantId=$${TENANT_ID} \
	--set ocmirrorImage="arohcpdev.azurecr.io/image-sync/ocmirror" \
	--set ocmirrorTag=latest \
	--set credsPullSecret=pull-secret \
	--set credsKeyVaultName=service-kv-aro-hcp-dev

deploy: deploy-shared deploy-component-sync deploy-oc-mirror

undeploy-shared:
	helm uninstall shared-resources --namespace ${NAMESPACE}

undeploy-oc-mirror:
	helm uninstall oc-mirror-cronjob --namespace ${NAMESPACE}

undeploy: undeploy-shared undeploy-oc-mirror

deploy-component-sync:
	az deployment group create --name comp-sync \
		--resource-group ${IMAGE_SYNC_RESOURCEGROUP} \
		--template-file componentSync/component-sync.bicep \
		--parameters componentSync/mvp-component-sync.bicepparam \
		--parameters containerImage="arohcpdev.azurecr.io/image-sync/component-sync:latest"

.PHONY: deploy-component-sync deploy-shared deploy-oc-mirror undeploy-shared undeploy-oc-mirror
