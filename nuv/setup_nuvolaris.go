// Licensed to the Apache Software Foundation (ASF) under one
// or more contributor license agreements.  See the NOTICE file
// distributed with this work for additional information
// regarding copyright ownership.  The ASF licenses this file
// to you under the Apache License, Version 2.0 (the
// "License"); you may not use this file except in compliance
// with the License.  You may obtain a copy of the License at
//
//   http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing,
// software distributed under the License is distributed on an
// "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
// KIND, either express or implied.  See the License for the
// specific language governing permissions and limitations
// under the License.
//
package main

import (
	"fmt"
	"os"
	"time"
)

type SetupPipeline struct {
	kubeClient          *KubeClient
	k8sContext          string
	apiHost             string
	operatorDockerImage string
	err                 error
	logger              *Logger
}

type setupStep func(sp *SetupPipeline)

func (sp *SetupPipeline) step(f setupStep) {
	if sp.err != nil {
		return
	}
	f(sp)
	time.Sleep(2 * time.Second)
}

func setupNuvolaris(logger *Logger, cmd *SetupCmd) error {

	setupWithNoFlags := !cmd.Devcluster &&
		!cmd.Configure &&
		cmd.Uninstall == "" &&
		cmd.Context == ""

	if setupWithNoFlags {
		err := listAvailableContexts()
		if err != nil {
			return err
		}
		return nil
	}

	if cmd.Configure {
		err := checkApiHost(cmd)
		if err != nil {
			return err
		}
		return configureCrd(cmd.Apihost)
	}

	sp := SetupPipeline{
		operatorDockerImage: cmd.OperatorImage + ":" + cmd.OperatorTag,
		logger:              logger,
	}

	if cmd.Devcluster {
		sp.err = startDevCluster(sp.logger)
		sp.k8sContext = "kind-nuvolaris"
	} else if cmd.Context == "" {
		fmt.Println("Specify Kubernetes context with --context flag")
		return nil
	}

	if cmd.Context != "" {
		err := checkApiHost(cmd)
		if err != nil {
			return err
		}
		sp.k8sContext = cmd.Context
		sp.apiHost = cmd.Apihost
	}

	if cmd.Uninstall != "" {
		sp.k8sContext = cmd.Uninstall
		sp.kubeClient, sp.err = initClients(sp.k8sContext)
		sp.step(resetNuvolaris)
	} else {
		sp.kubeClient, sp.err = initClients(sp.k8sContext)
		sp.step(createNuvolarisNamespace)
		sp.step(deployServiceAccount)
		sp.step(deployClusterRoleBinding)
		sp.step(runNuvolarisOperatorPod)
		sp.step(waitForCrdDefinitionReady)
		sp.step(deployOperatorObject)
		sp.step(waitForOpenWhiskReady)
		sp.step(manifestDeploy)
	}
	return sp.err
}

func checkApiHost(cmd *SetupCmd) error {
	if cmd.Apihost == "" {
		return fmt.Errorf("please specify the public IP of your Kubernetes cluster - if your Kubernetes has a load balancer, specify --apihost=auto")
	}
	return nil
}

func createNuvolarisNamespace(sp *SetupPipeline) {
	sp.err = sp.kubeClient.createNuvolarisNamespace()
}

func deployServiceAccount(sp *SetupPipeline) {
	sp.err = sp.kubeClient.createServiceAccount()
}

func deployClusterRoleBinding(sp *SetupPipeline) {
	sp.err = sp.kubeClient.createClusterRoleBinding()
}

func runNuvolarisOperatorPod(sp *SetupPipeline) {
	sp.err = sp.kubeClient.createOperatorPod(sp.operatorDockerImage)
}

func waitForCrdDefinitionReady(sp *SetupPipeline) {
	sp.err = crdProbe(sp.kubeClient)
}

func deployOperatorObject(sp *SetupPipeline) {
	sp.err = createWhiskOperatorObject(sp.kubeClient, sp.apiHost)
}

func waitForOpenWhiskReady(sp *SetupPipeline) {
	sp.err = readinessProbe(sp.kubeClient)
}

func manifestDeploy(sp *SetupPipeline) {
	if _, err := os.Stat("./manifest.yaml"); err == nil {
		fmt.Println("Deploying manifest.yaml")
		sp.err = Wsk([]string{"wsk", "project", "deploy"})
	} else {
		sp.err = nil
	}
}

func resetNuvolaris(sp *SetupPipeline) {
	sp.err = sp.kubeClient.cleanup()
}
