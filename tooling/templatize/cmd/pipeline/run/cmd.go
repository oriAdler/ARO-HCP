package run

import (
	"context"
	"fmt"
	"os/exec"
	"strings"

	"github.com/Masterminds/semver/v3"
	"github.com/spf13/cobra"

	"github.com/Azure/ARO-HCP/tooling/templatize/pkg/azauth"
)

type Version struct {
	Cmd        string
	Name       string
	Constraint string
}

var versionConstraints = []Version{
	{
		Name:       "Azure CLI",
		Cmd:        "az --version | grep azure-cli |awk '{print $NF}'",
		Constraint: ">=2.68.0",
	},
	{
		Name:       "bicep module",
		Cmd:        "az bicep version --only-show-errors |grep 'CLI version' |awk '{print $4}'",
		Constraint: ">=0.31.0",
	},
}

func NewCommand() (*cobra.Command, error) {
	opts := DefaultOptions()
	cmd := &cobra.Command{
		Use:   "run",
		Short: "run a pipeline.yaml file towards an Azure Resourcegroup / AKS cluster",
		Long:  "run a pipeline.yaml file towards an Azure Resourcegroup / AKS cluster",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runPipeline(cmd.Context(), opts)
		},
	}
	if err := BindOptions(opts, cmd); err != nil {
		return nil, err
	}
	return cmd, nil
}

func ensureDependencies(ctx context.Context) error {
	for _, c := range versionConstraints {
		fmt.Println("Checking verion of", c.Name)
		cmd := exec.CommandContext(ctx, "/bin/bash", "-c", c.Cmd)
		output, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("Error checking version %s, error: %v", c.Name, err)
		}
		semverConstraint, err := semver.NewConstraint(c.Constraint)
		if err != nil {
			return fmt.Errorf("Error creation version constraint %v", err)
		}
		trimmedOutput := strings.TrimSuffix(string(output), "\n")
		v, err := semver.NewVersion(trimmedOutput)
		if err != nil {
			return fmt.Errorf("Error parsing version '%s' %v", trimmedOutput, err)
		}

		if !semverConstraint.Check(v) {
			return fmt.Errorf("wrong version, expected '%s', found: '%s'", c.Constraint, trimmedOutput)
		}

	}
	return nil
}

func runPipeline(ctx context.Context, opts *RawRunOptions) error {
	validated, err := opts.Validate()
	if err != nil {
		return err
	}
	completed, err := validated.Complete()
	if err != nil {
		return err
	}
	err = azauth.SetupAzureAuth(ctx)
	if err != nil {
		return err
	}
	err = ensureDependencies(ctx)
	if err != nil {
		return err
	}
	return completed.RunPipeline(ctx)
}
