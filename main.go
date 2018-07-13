package main

import (
	"encoding/base64"
	"fmt"
	"io/ioutil"
	"strings"

	sdk "github.com/gaia-pipeline/gosdk"
	"k8s.io/api/core/v1"
	"k8s.io/api/extensions/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	vaultAddress        = "http://localhost:8200"
	vaultToken          = "root-token"
	kubeConfVaultPath   = "secret/data/kube-conf"
	appVersionVaultPath = "secret/data/nginx"
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
	conf := s.Data["data"].(map[string]interface{})
	kubeConf, err := base64.StdEncoding.DecodeString(conf["conf"].(string))
	if err != nil {
		return err
	}

	// Convert config to string and replace localhost.
	// We use here the magical DNS name "host.docker.internal",
	// which resolves to the internal IP address used by the host.
	// If this should not work for you, replace it with your real IP address.
	confStr := string(kubeConf[:])
	confStr = strings.Replace(confStr, "localhost", "host.docker.internal", 1)
	kubeConf = []byte(confStr)

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
	version := (v.Data["data"].(map[string]interface{}))["version"].(string)
	if err = writeToFile(appVersionLocalPath, []byte(version)); err != nil {
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
		ObjectMeta: metav1.ObjectMeta{
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
	d := v1beta1.Deployment{}
	d.ObjectMeta = metav1.ObjectMeta{
		Name: appName,
		Labels: map[string]string{
			"app": appName,
		},
	}
	d.Spec = v1beta1.DeploymentSpec{
		Replicas: &replicas,
		Selector: &metav1.LabelSelector{
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
	deployClient := c.ExtensionsV1beta1().Deployments(appName)
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
