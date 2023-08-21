// SPDX-License-Identifier: Apache-2.0
// Copyright Authors of Cilium

//go:build ipam_provider_oci

package cmd

import (
	operatorOption "github.com/cilium/cilium/operator/option"
	"github.com/cilium/cilium/pkg/option"
)

func init() {
	flags := rootCmd.Flags()

	flags.String(operatorOption.OCIVCNID, "", "Specific VCN ID for OCI ENI. If not set use same VCN as operator")
	option.BindEnv(Vp, operatorOption.OCIVCNID)

	Vp.BindPFlags(flags)
}
