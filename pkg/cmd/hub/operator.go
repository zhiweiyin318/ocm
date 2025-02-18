package hub

import (
	"github.com/openshift/library-go/pkg/controller/controllercmd"
	"github.com/spf13/cobra"

	"open-cluster-management.io/ocm/pkg/operator/operators/clustermanager"
	"open-cluster-management.io/ocm/pkg/version"
)

// NewHubOperatorCmd generatee a command to start hub operator
func NewHubOperatorCmd() *cobra.Command {

	options := clustermanager.Options{}
	cmd := controllercmd.
		NewControllerCommandConfig("clustermanager", version.Get(), options.RunClusterManagerOperator).
		NewCommand()
	cmd.Use = "hub"
	cmd.Short = "Start the cluster manager operator"

	cmd.Flags().BoolVar(&options.SkipRemoveCRDs, "skip-remove-crds", false, "Skip removing CRDs while ClusterManager is deleting.")
	return cmd
}
