// SPDX-License-Identifier: Apache-2.0
// Copyright Authors of Cilium

//go:build ipam_provider_oci

package cmd

import (
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	operatorOption "github.com/cilium/cilium/operator/option"
	"github.com/cilium/cilium/pkg/option"
)

func init() {
	FlagsHooks = append(FlagsHooks, &ociFlagsHooks{})
}

type ociFlagsHooks struct{}

func (h *ociFlagsHooks) RegisterProviderFlag(cmd *cobra.Command, vp *viper.Viper) {
	flags := cmd.Flags()

	flags.String(operatorOption.OCIVCNID, "", "Specific VCN ID for OCI ENI. If not set use same VCN as operator")
	option.BindEnv(vp, operatorOption.OCIVCNID)

	flags.Bool(operatorOption.OCIUseInstancePrincipal, true, "Use instance principal authentication for OCI (default true, set to false to use config file)")
	option.BindEnv(vp, operatorOption.OCIUseInstancePrincipal)

	vp.BindPFlags(flags)
}
