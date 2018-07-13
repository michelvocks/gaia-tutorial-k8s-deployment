package main

import (
	"encoding/base64"
	"fmt"
	"io/ioutil"

	sdk "github.com/gaia-pipeline/gosdk"
	"k8s.io/client-go/pkg/api/unversioned"
	"k8s.io/client-go/pkg/api/v1"
	extensionsv1beta1 "k8s.io/client-go/pkg/apis/extensions/v1beta1"
)

const (
	vaultAddress        = "http://dev-vault:8200"
	vaultToken          = "root-token"
	kubeConfVaultPath   = "secret/kube-config"
	appVersionVaultPath = "secret/nginx"
	kubeLocalPath       = "/tmp/kube-conf"
	appVersionLocalPath = "/tmp/app-version"

	// Deployment specific attributes
	appName = "nginx"
)

var (
	replicas int32 = 2
)

// GetSecretsFromVault retrieves all information and credentials
// from vault and saves it to local space.
func GetSecretsFromVault() error {
	// Create vault client
	vaultClient, err := connectToVault()
	if err != nil {
		return err
	}

	// Read kube config from vault and decode base64
	l := vaultClient.Logical()
	s, err := l.Read(kubeConfVaultPath)
	if err != nil {
		return err
	}
	kubeConf, err := base64.StdEncoding.DecodeString(s.Data["conf"].(string))
	if err != nil {
		return err
	}

	// Write kube config to file
	if err = writeToFile(kubeLocalPath, kubeConf); err != nil {
		return err
	}

	// Read app image version from vault
	v, err := l.Read(appVersionVaultPath)
	if err != nil {
		return err
	}

	// Write image version to file
	if err = writeToFile(appVersionLocalPath, v.Data["version"]); err != nil {
		return err
	}
	return nil
}

// CreateNamespace creates the namespace for our app.
func CreateNamespace() error {
	// Get kubernetes client
	c, err := getKubeClient(kubeLocalPath)
	if err != nil {
		return err
	}

	// Create namespace object
	ns := &v1.Namespace{
		ObjectMeta: v1.ObjectMeta{
			Name: appName,
		},
	}

	// Create namespace
	_, err = c.Core().Namespaces().Create(ns)
	return err
}

// CreateDeployment retrieves information from vault like
// kube config and image version. Then it creates the
// kubernetes deployment object.
func CreateDeployment() error {
	// Get kubernetes client
	c, err := getKubeClient(kubeLocalPath)
	if err != nil {
		return err
	}

	// Load image version from file
	v, err := ioutil.ReadFile(appVersionLocalPath)
	if err != nil {
		return err
	}

	// Create deployment object
	d := extensionsv1beta1.Deployment{}
	d.ObjectMeta = v1.ObjectMeta{
		Name: appName,
		Labels: map[string]string{
			"app": appName,
		},
	}
	d.Spec = extensionsv1beta1.DeploymentSpec{
		Replicas: &replicas,
		Selector: &unversioned.LabelSelector{
			MatchLabels: map[string]string{
				"app": appName,
			},
		},
		Template: v1.PodTemplateSpec{
			ObjectMeta: d.ObjectMeta,
			Spec: v1.PodSpec{
				Containers: []v1.Container{
					v1.Container{
						Name:            appName,
						Image:           fmt.Sprintf("%s:%s", appName, v),
						ImagePullPolicy: v1.PullAlways,
						Ports: []v1.ContainerPort{
							v1.ContainerPort{
								ContainerPort: int32(80),
							},
						},
					},
				},
			},
		},
	}

	// Create deployment object in kubernetes
	deployClient := c.ExtensionsV1beta1Client.Deployments(appName)
	_, err = deployClient.Create(&d)
	return err
}

func main() {
	jobs := sdk.Jobs{
		sdk.Job{
			Handler:     GetSecretsFromVault,
			Title:       "Get secrets from vault",
			Description: "Get secrets from vault",
			Priority:    0,
		},
		sdk.Job{
			Handler:     CreateNamespace,
			Title:       "Create kubernetes namespace",
			Description: "Create kubernetes namespace if not exist",
			Priority:    10,
		},
		sdk.Job{
			Handler:     CreateDeployment,
			Title:       "Create kubernetes app deployment",
			Description: "Create kubernetes app deplyment",
			Priority:    20,
		},
	}

	// Serve
	if err := sdk.Serve(jobs); err != nil {
		panic(err)
	}
}
