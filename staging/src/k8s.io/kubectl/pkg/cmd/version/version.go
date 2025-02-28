/*
Copyright 2014 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package version

import (
	"encoding/json"
	"errors"
	"fmt"
	"runtime/debug"

	"github.com/spf13/cobra"
	"sigs.k8s.io/yaml"

	apimachineryversion "k8s.io/apimachinery/pkg/version"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/component-base/version"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
	"k8s.io/kubectl/pkg/util/i18n"
	"k8s.io/kubectl/pkg/util/templates"
)

// TODO(knverey): remove this hardcoding once kubectl being built with module support makes BuildInfo available.
const kustomizeVersion = "v4.4.1"

// Version is a struct for version information
type Version struct {
	ClientVersion    *apimachineryversion.Info `json:"clientVersion,omitempty" yaml:"clientVersion,omitempty"`
	KustomizeVersion string                    `json:"kustomizeVersion,omitempty" yaml:"kustomizeVersion,omitempty"`
	ServerVersion    *apimachineryversion.Info `json:"serverVersion,omitempty" yaml:"serverVersion,omitempty"`
}

var (
	versionExample = templates.Examples(i18n.T(`
		# Print the client and server versions for the current context
		kubectl version`))
)

// Options is a struct to support version command
type Options struct {
	ClientOnly bool
	Short      bool
	Output     string

	discoveryClient discovery.CachedDiscoveryInterface

	genericclioptions.IOStreams
}

// NewOptions returns initialized Options
func NewOptions(ioStreams genericclioptions.IOStreams) *Options {
	return &Options{
		IOStreams: ioStreams,
	}

}

// NewCmdVersion returns a cobra command for fetching versions
func NewCmdVersion(f cmdutil.Factory, ioStreams genericclioptions.IOStreams) *cobra.Command {
	o := NewOptions(ioStreams)
	cmd := &cobra.Command{
		Use:     "version",
		Short:   i18n.T("Print the client and server version information"),
		Long:    i18n.T("Print the client and server version information for the current context."),
		Example: versionExample,
		Run: func(cmd *cobra.Command, args []string) {
			cmdutil.CheckErr(o.Complete(f, cmd))
			cmdutil.CheckErr(o.Validate(args))
			cmdutil.CheckErr(o.Run())
		},
	}
	cmd.Flags().BoolVar(&o.ClientOnly, "client", o.ClientOnly, "If true, shows client version only (no server required).")
	cmd.Flags().BoolVar(&o.Short, "short", o.Short, "If true, print just the version number.")
	cmd.Flags().StringVarP(&o.Output, "output", "o", o.Output, "One of 'yaml' or 'json'.")
	return cmd
}

// Complete completes all the required options
func (o *Options) Complete(f cmdutil.Factory, cmd *cobra.Command) error {
	var err error
	if o.ClientOnly {
		return nil
	}
	o.discoveryClient, err = f.ToDiscoveryClient()
	// if we had an empty rest.Config, continue and just print out client information.
	// if we had an error other than being unable to build a rest.Config, fail.
	if err != nil && !clientcmd.IsEmptyConfig(err) {
		return err
	}
	return nil
}

// Validate validates the provided options
func (o *Options) Validate(args []string) error {
	if len(args) != 0 {
		return errors.New(fmt.Sprintf("extra arguments: %v", args))
	}

	if o.Output != "" && o.Output != "yaml" && o.Output != "json" {
		return errors.New(`--output must be 'yaml' or 'json'`)
	}

	return nil
}

// Run executes version command
func (o *Options) Run() error {
	var (
		serverErr   error
		versionInfo Version
	)

	versionInfo.ClientVersion = func() *apimachineryversion.Info { v := version.Get(); return &v }()
	versionInfo.KustomizeVersion = getKustomizeVersion()

	if !o.ClientOnly && o.discoveryClient != nil {
		// Always request fresh data from the server
		o.discoveryClient.Invalidate()
		versionInfo.ServerVersion, serverErr = o.discoveryClient.ServerVersion()
	}

	switch o.Output {
	case "":
		if o.Short {
			fmt.Fprintf(o.Out, "Client Version: %s\n", versionInfo.ClientVersion.GitVersion)
			fmt.Fprintf(o.Out, "Kustomize Version: %s\n", versionInfo.KustomizeVersion)
			if versionInfo.ServerVersion != nil {
				fmt.Fprintf(o.Out, "Server Version: %s\n", versionInfo.ServerVersion.GitVersion)
			}
		} else {
			fmt.Fprintf(o.Out, "Client Version: %#v\n", *versionInfo.ClientVersion)
			fmt.Fprintf(o.Out, "Kustomize Version: %s\n", versionInfo.KustomizeVersion)
			if versionInfo.ServerVersion != nil {
				fmt.Fprintf(o.Out, "Server Version: %#v\n", *versionInfo.ServerVersion)
			}
		}
	case "yaml":
		marshalled, err := yaml.Marshal(&versionInfo)
		if err != nil {
			return err
		}
		fmt.Fprintln(o.Out, string(marshalled))
	case "json":
		marshalled, err := json.MarshalIndent(&versionInfo, "", "  ")
		if err != nil {
			return err
		}
		fmt.Fprintln(o.Out, string(marshalled))
	default:
		// There is a bug in the program if we hit this case.
		// However, we follow a policy of never panicking.
		return fmt.Errorf("VersionOptions were not validated: --output=%q should have been rejected", o.Output)
	}

	if versionInfo.ServerVersion != nil {
		if err := printVersionSkewWarning(o.ErrOut, *versionInfo.ClientVersion, *versionInfo.ServerVersion); err != nil {
			return err
		}
	}

	return serverErr
}

func getKustomizeVersion() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return kustomizeVersion
	}
	for _, dep := range info.Deps {
		if dep.Path == "sigs.k8s.io/kustomize/kustomize/v4" {
			return dep.Version
		}
	}
	return kustomizeVersion
}
