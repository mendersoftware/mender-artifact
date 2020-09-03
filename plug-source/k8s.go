package main

import (
	"fmt"

	"github.com/urfave/cli"
	"os/exec"
	"github.com/pkg/errors"
)

func generateK8sArtifact(c *cli.Context) (err error) {
	var cmd *exec.Cmd
	for _, manifesto := range c.StringSlice("manifestos") {
		cmd = exec.Command("kubectl", "apply", "--dry-run", "-f", manifesto)
		if err = cmd.Run(); err != nil {
			return errors.Wrapf(err, "The manifesto: %q is not a valid manifesto. Aborting",
				manifesto)
		}
	}
	fmt.Println("wrote k8s update-module artifact")
	return nil
}

func CLI() cli.Command {
	writeModuleCommand := cli.Command{
		Name:      "k8s",
		Action:    generateK8sArtifact,
		Usage:     "Writes Mender artifact for an k8s update module",
		UsageText: "Writes a k8s Mender artifact that will be used by the k8s update module. ",
		Flags: []cli.Flag{
			cli.StringSliceFlag{
				Name:     "manifestos, ma",
				Usage:    "Path(s) to the Kubernetes manifesto(s)",
				Required: true,
			},
		},
	}

	return writeModuleCommand
}
