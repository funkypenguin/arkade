// Copyright (c) arkade author(s) 2022. All rights reserved.
// Licensed under the MIT license. See LICENSE file in the project root for full license information.

package apps

import (
	"errors"
	"fmt"
	"log"
	"os"
	"path"
	"strconv"

	"golang.org/x/crypto/bcrypt"

	"github.com/alexellis/arkade/pkg"
	"github.com/alexellis/arkade/pkg/config"
	"github.com/alexellis/arkade/pkg/env"
	"github.com/alexellis/arkade/pkg/helm"
	"github.com/alexellis/arkade/pkg/k8s"
	"github.com/sethvargo/go-password/password"
	"github.com/spf13/cobra"
)

func MakeInstallRegistry() *cobra.Command {
	var registry = &cobra.Command{
		Use:   "docker-registry",
		Short: "Install a community maintained Docker registry chart",
		Long: `Install a community maintained Docker registry chart for more info
Check out the project's repo https://github.com/twuni/docker-registry.helm
Or the Open Source Docker registry https://github.com/distribution/distribution`,
		Example:      `  arkade install registry --namespace default`,
		SilenceUsage: true,
	}

	registry.Flags().StringP("namespace", "n", "default", "The namespace used for installation")
	registry.Flags().Bool("update-repo", true, "Update the helm repo")
	registry.Flags().StringP("username", "u", "admin", "Username for the registry")
	registry.Flags().StringP("password", "p", "", "Password for the registry, leave blank to generate")
	registry.Flags().StringP("write-file", "w", "", "Write generated password to this file")
	registry.Flags().Bool("persistence", false, "Enable persistence")
	registry.Flags().StringArray("set", []string{},
		"Use custom flags or override existing flags \n(example --set persistence.enabled=true)")

	registry.RunE = func(command *cobra.Command, args []string) error {
		kubeConfigPath, _ := command.Flags().GetString("kubeconfig")

		if err := config.SetKubeconfig(kubeConfigPath); err != nil {
			return err
		}
		wait, _ := command.Flags().GetBool("wait")

		updateRepo, _ := registry.Flags().GetBool("update-repo")

		userPath, err := config.InitUserDir()
		if err != nil {
			return err
		}
		namespace, _ := command.Flags().GetString("namespace")
		if namespace != "default" {
			return errors.New(`to override the "default", install via helm directly`)
		}

		outputFile, _ := command.Flags().GetString("write-file")

		clientArch, clientOS := env.GetClientArch()

		fmt.Printf("Client: %s, %s\n", clientArch, clientOS)
		log.Printf("User dir established as: %s\n", userPath)

		os.Setenv("HELM_HOME", path.Join(userPath, ".helm"))

		_, err = helm.TryDownloadHelm(userPath, clientArch, clientOS)
		if err != nil {
			return err
		}

		username, _ := command.Flags().GetString("username")

		pass, _ := command.Flags().GetString("password")
		if len(pass) == 0 {
			key, err := password.Generate(20, 10, 0, false, true)
			if err != nil {
				return err
			}

			pass = key
		}

		val, err := bcrypt.GenerateFromPassword([]byte(pass), bcrypt.DefaultCost)

		if err != nil {
			return err
		}

		htPasswd := fmt.Sprintf("%s:%s\n", username, string(val))

		err = helm.AddHelmRepo("twuni", "https://twuni.github.io/docker-registry.helm", updateRepo)
		if err != nil {
			return err
		}

		chartPath := path.Join(os.TempDir(), "charts")
		err = helm.FetchChart("twuni/docker-registry", defaultVersion)

		if err != nil {
			return err
		}

		persistence, err := registry.Flags().GetBool("persistence")
		if err != nil {
			return err
		}

		overrides := map[string]string{}

		overrides["persistence.enabled"] = strconv.FormatBool(persistence)
		overrides["secrets.htpasswd"] = htPasswd

		customFlags, err := command.Flags().GetStringArray("set")
		if err != nil {
			return err
		}

		if err = config.MergeFlags(overrides, customFlags); err != nil {
			return err
		}

		arch := k8s.GetNodeArchitecture()
		fmt.Printf("Node architecture: %q\n", arch)

		fmt.Println("Chart path: ", chartPath)

		ns := "default"

		err = helm.Helm3Upgrade("twuni/docker-registry", ns,
			"values.yaml",
			defaultVersion,
			overrides,
			wait)

		if err != nil {
			return err
		}

		fmt.Println(registryInstallMsg)

		if len(outputFile) > 0 {
			err := os.WriteFile(outputFile, []byte(pass), 0600)
			if err != nil {
				return err
			}

			fmt.Printf("See %s for credentials\n", outputFile)
		} else {
			fmt.Printf("Registry credentials: %s %s\nexport PASSWORD=%s\n", username, pass, pass)
		}

		return nil
	}

	return registry
}

const RegistryInfoMsg = `# Your docker-registry has been configured

kubectl logs deploy/docker-registry

export IP="192.168.0.11" # Set to WiFI/ethernet adapter
export PASSWORD="" # See below
kubectl port-forward svc/docker-registry --address 0.0.0.0 5000 &

docker login $IP:5000 --username admin --password $PASSWORD
docker tag alpine:3.11 $IP:5000/alpine:3.11
docker push $IP:5000/alpine:3.11

# This chart is community maintained.
# Find out more at:
# https://github.com/twuni/docker-registry.helm
# https://github.com/distribution/distribution`

const registryInstallMsg = `=======================================================================
= docker-registry has been installed.                                 =
=======================================================================` +
	"\n\n" + RegistryInfoMsg + "\n\n" + pkg.SupportMessageShort
