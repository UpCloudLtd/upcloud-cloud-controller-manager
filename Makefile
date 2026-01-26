# Ensure Make is run with bash shell as some syntax below is bash-specific
SHELL:=/usr/bin/env bash


ROOT_DIR:=$(shell dirname $(realpath $(firstword $(MAKEFILE_LIST))))
BIN_DIR := $(abspath $(ROOT_DIR)/bin)
GOPROXY := $(shell go env GOPROXY)
ifeq ($(GOPROXY),)
GOPROXY := https://proxy.golang.org
endif
export GOPROXY

export LDFLAGS := "-w -s -X 'k8s.io/component-base/version/verflag.programName=UpCloud cloud controller manager' $(shell scripts/version.sh "ldflags")"
## --------------------------------------
## Binaries
## --------------------------------------

.PHONY: manager
manager: ## Build cloud controller manager binary in local environment
	CGO_ENABLED=0 GOPROXY=$(GOPROXY) go build -ldflags $(LDFLAGS) -o $(BIN_DIR)/cloud-controller-manager github.com/UpCloudLtd/upcloud-cloud-controller-manager/cmd/upcloud-cloud-controller-manager
