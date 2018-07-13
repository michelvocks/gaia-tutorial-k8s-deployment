package main

import "testing"

func TestCreateService(t *testing.T) {
	GetSecretsFromVault()
	CreateService()
}
