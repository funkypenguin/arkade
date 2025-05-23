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

	"github.com/alexellis/arkade/pkg/k8s"

	"github.com/alexellis/arkade/pkg"
	"github.com/alexellis/arkade/pkg/config"
	"github.com/alexellis/arkade/pkg/env"
	"github.com/alexellis/arkade/pkg/helm"
	"github.com/spf13/cobra"
)

func MakeInstallMongoDB() *cobra.Command {
	var command = &cobra.Command{
		Use:          "mongodb",
		Short:        "Install mongodb",
		Long:         `Install mongodb`,
		Example:      `  arkade install mongodb`,
		SilenceUsage: true,
	}
	command.Flags().String("namespace", "default", "Namespace for the app")

	command.Flags().StringArray("set", []string{},
		"Use custom flags or override existing flags \n(example --set mongodbUsername=admin)")
	command.Flags().Bool("persistence", false, "Create and bind a persistent volume, not recommended for development")

	command.RunE = func(command *cobra.Command, args []string) error {
		kubeConfigPath, _ := command.Flags().GetString("kubeconfig")
		if err := config.SetKubeconfig(kubeConfigPath); err != nil {
			return err
		}
		wait, _ := command.Flags().GetBool("wait")

		namespace, _ := command.Flags().GetString("namespace")

		arch := k8s.GetNodeArchitecture()
		fmt.Printf("Node architecture: %q\n", arch)

		if arch != IntelArch {
			return errors.New(OnlyIntelArch)
		}

		userPath, err := config.InitUserDir()
		if err != nil {
			return err
		}

		clientArch, clientOS := env.GetClientArch()

		fmt.Printf("Client: %q, %q\n", clientArch, clientOS)

		log.Printf("User dir established as: %s\n", userPath)

		os.Setenv("HELM_HOME", path.Join(userPath, ".helm"))

		persistence, _ := command.Flags().GetBool("persistence")

		_, err = helm.TryDownloadHelm(userPath, clientArch, clientOS)
		if err != nil {
			return err
		}

		updateRepo, _ := command.Flags().GetBool("update-repo")
		err = helm.AddHelmRepo("bitnami", "https://charts.bitnami.com/bitnami", updateRepo)
		if err != nil {
			return fmt.Errorf("unable to add repo %s", err)
		}

		err = helm.FetchChart("bitnami/mongodb", defaultVersion)

		if err != nil {
			return fmt.Errorf("unable fetch chart %s", err)
		}

		overrides := map[string]string{}

		overrides["persistence.enabled"] = strconv.FormatBool(persistence)

		customFlags, err := command.Flags().GetStringArray("set")
		if err != nil {
			return fmt.Errorf("error with --set usage: %s", err)
		}

		if err := config.MergeFlags(overrides, customFlags); err != nil {
			return err
		}

		err = helm.Helm3Upgrade("bitnami/mongodb",
			namespace, "values.yaml", defaultVersion, overrides, wait)
		if err != nil {
			return fmt.Errorf("unable to mongodb chart with helm %s", err)
		}
		fmt.Println(mongoDBPostInstallMsg)
		return nil
	}
	return command
}

const mongoDBPostInstallMsg = `=======================================================================
=                  MongoDB has been installed.                        =
=======================================================================` +
	"\n\n" + pkg.SupportMessageShort

var MongoDBInfoMsg = `
# MongoDB can be accessed via port 27017 on the following DNS name from within your cluster:

mongodb.{{namespace}}.svc.cluster.local

# To get the root password run:

export MONGODB_ROOT_PASSWORD=$(kubectl get secret --namespace {{namespace}} mongodb -o jsonpath="{.data.mongodb-root-password}" | base64 --decode)

# To connect to your database run the following command:

kubectl run --namespace {{namespace}} mongodb-client --rm --tty -i --restart='Never' --image bitnami/mongodb --command -- mongo admin --host mongodb --authenticationDatabase admin -u root -p $MONGODB_ROOT_PASSWORD

# To connect to your database from outside the cluster execute the following commands:

kubectl port-forward --namespace {{namespace}} svc/mongodb 27017:27017 &
mongo --host 127.0.0.1 --authenticationDatabase admin -p $MONGODB_ROOT_PASSWORD

# More on GitHub : https://github.com/helm/charts/tree/master/stable/mongodb`
