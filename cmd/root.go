/***************************************************************
 *
 * Copyright (C) 2023, Pelican Project, Morgridge Institute for Research
 *
 * Licensed under the Apache License, Version 2.0 (the "License"); you
 * may not use this file except in compliance with the License.  You may
 * obtain a copy of the License at
 *
 *    http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 *
 ***************************************************************/

package main

import (
	"os"
	"strconv"
	"strings"

	"github.com/pelicanplatform/pelican/config"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

type uint16Value uint16

var (
	cfgFile    string
	outputJSON bool

	rootCmd = &cobra.Command{
		Use:   "pelican",
		Short: "Interact with data federations",
		Long: `The pelican software allows one to build and interact
with data federations, enabling the sharing of objects and collections
across multiple dataset providers.`,
	}

	// We want the value of this port flag to correspond to the Port viper key.
	// However, only one flag pointer can correspond to the key.  If we define this
	// in `pelican registry serve` and `pelican director serve`, then whatever init()
	// function is run second will be the only one that is set (the first definition
	// of the flag is overwritten and thus ignored).
	//
	// Accordingly, we define the flag once globally and strategically insert the
	// pointer into any command that may want to define the port.
	emptyPort = uint16(0)
	portFlag  = &pflag.Flag{
		Name:      "port",
		Shorthand: "p",
		Usage:     "Set the port at which the web server should be accessible",
		Value:     (*uint16Value)(&emptyPort),
	}
)

// The Value member of the portFlag object must implement the pflag.Value interface.
// Unfortunately, the pflag module does not currently export any types that implement
// this interface so we have to reimplement it here.
func (i *uint16Value) Set(s string) error {
	v, err := strconv.ParseUint(s, 0, 16)
	*i = uint16Value(v)
	return err
}

func (i *uint16Value) Type() string {
	return "uint16"
}

func (i *uint16Value) String() string { return strconv.FormatUint(uint64(*i), 10) }

func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

func init() {

	cobra.OnInitialize(config.InitConfig)
	rootCmd.AddCommand(objectCmd)
	objectCmd.CompletionOptions.DisableDefaultCmd = true
	rootCmd.AddCommand(directorCmd)
	rootCmd.AddCommand(namespaceRegistryCmd)
	rootCmd.AddCommand(originCmd)
	rootCmd.AddCommand(cacheCmd)
	rootCmd.AddCommand(namespaceCmd)
	rootCmd.AddCommand(rootConfigCmd)
	rootCmd.AddCommand(rootPluginCmd)
	preferredPrefix := config.GetPreferredPrefix()
	rootCmd.Use = strings.ToLower(preferredPrefix)

	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is $HOME/.config/pelican/pelican.yaml)")

	rootCmd.PersistentFlags().BoolP("debug", "d", false, "Enable debug logs")

	rootCmd.PersistentFlags().StringP("federation", "f", "", "Pelican federation to utilize")
	if err := viper.BindPFlag("Federation.DiscoveryUrl", rootCmd.PersistentFlags().Lookup("federation")); err != nil {
		panic(err)
	}

	// Register the version flag here just so --help will show this flag
	// Actual checking is executed at main.go
	// Remove the shorthand -v since in "origin serve" flagset it's already used for "volume" flag
	rootCmd.PersistentFlags().BoolP("version", "", false, "Print the version and exit")

	rootCmd.PersistentFlags().BoolVarP(&outputJSON, "json", "", false, "output results in JSON format")
	rootCmd.CompletionOptions.DisableDefaultCmd = true

	if err := viper.BindPFlag("config", rootCmd.PersistentFlags().Lookup("config")); err != nil {
		panic(err)
	}
	if err := viper.BindPFlag("Debug", rootCmd.PersistentFlags().Lookup("debug")); err != nil {
		panic(err)
	}
	if err := viper.BindPFlag("Server.Port", portFlag); err != nil {
		panic(err)
	}
}
