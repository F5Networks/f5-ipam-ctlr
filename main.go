/*-
 * Copyright (c) 2018, F5 Networks, Inc.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *    http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	log "github.com/F5Networks/f5-ipam-ctlr/pkg/vlogger"
	clog "github.com/F5Networks/f5-ipam-ctlr/pkg/vlogger/console"
	flag "github.com/spf13/pflag"
	"golang.org/x/crypto/ssh/terminal"

	"github.com/F5Networks/f5-ipam-ctlr/pkg/controller"
	"github.com/F5Networks/f5-ipam-ctlr/pkg/orchestration"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

var (
	// To be set by build
	version   string
	buildInfo string

	// Flag sets and supported flags
	flags       *flag.FlagSet
	kubeFlags   *flag.FlagSet
	globalFlags *flag.FlagSet

	// Global
	logLevel     *string
	orch         *string
	printVersion *bool

	// Kubernetes
	inCluster      *bool
	kubeConfig     *string
	namespaces     *[]string
	namespaceLabel *string
)

func init() {
	flags = flag.NewFlagSet("main", flag.ContinueOnError)
	globalFlags = flag.NewFlagSet("Global", flag.ContinueOnError)
	kubeFlags = flag.NewFlagSet("Kubernetes", flag.ContinueOnError)

	// Flag terminal wrapping
	var err error
	var width int
	fd := int(os.Stdout.Fd())
	if terminal.IsTerminal(fd) {
		width, _, err = terminal.GetSize(fd)
		if nil != err {
			width = 0
		}
	}

	// Global flags
	logLevel = globalFlags.String("log-level", "INFO", "Optional, logging level.")
	orch = globalFlags.String("orchestration", "",
		"Required, orchestration that the controller is running in.")
	printVersion = globalFlags.Bool("version", false, "Optional, print version and exit.")

	// Kubernetes flags
	inCluster = kubeFlags.Bool("running-in-cluster", true,
		"Optional, if this controller is running in a Kubernetes cluster, "+
			"use the pod secrets for creating a Kubernetes client.")
	kubeConfig = kubeFlags.String("kubeconfig", "./config",
		"Optional, absolute path to the kubeconfig file.")
	namespaces = kubeFlags.StringArray("namespace", []string{},
		"Optional, Kubernetes namespace(s) to watch. "+
			"If left blank controller will watch all k8s namespaces.")
	namespaceLabel = kubeFlags.String("namespace-label", "",
		"Optional, used to watch for namespaces with this label.")

	globalFlags.Usage = func() {
		fmt.Fprintf(os.Stderr, "  Global:\n%s\n", globalFlags.FlagUsagesWrapped(width))
	}
	kubeFlags.Usage = func() {
		fmt.Fprintf(os.Stderr, "  Kubernetes:\n%s\n", kubeFlags.FlagUsagesWrapped(width))
	}
	flags.AddFlagSet(globalFlags)
	flags.AddFlagSet(kubeFlags)

	flags.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage of %s\n", os.Args[0])
		globalFlags.Usage()
		kubeFlags.Usage()
	}
}

func verifyArgs() error {
	log.RegisterLogger(
		log.LL_MIN_LEVEL, log.LL_MAX_LEVEL, clog.NewConsoleLogger())

	if ll := log.NewLogLevel(*logLevel); nil != ll {
		log.SetLogLevel(*ll)
	} else {
		return fmt.Errorf("Unknown log level requested: %v\n"+
			"    Valid log levels are: DEBUG, INFO, WARNING, ERROR, CRITICAL", logLevel)
	}

	if len(*orch) == 0 {
		return fmt.Errorf("Orchestration is required.")
	}

	if len(*namespaces) != 0 && len(*namespaceLabel) != 0 {
		return fmt.Errorf("Cannot specify both namespace and namespace-label.")
	}

	return nil
}

func main() {
	err := flags.Parse(os.Args)
	if nil != err {
		os.Exit(1)
	}

	if *printVersion {
		fmt.Printf("Version: %s\nBuild: %s\n", version, buildInfo)
		os.Exit(0)
	}

	err = verifyArgs()
	if nil != err {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		flags.Usage()
		os.Exit(1)
	}

	log.Infof("Starting: Version: %s, BuildInfo: %s", version, buildInfo)

	stopCh := make(chan struct{})

	// Create a channel for the orchestration client to send data to the controller
	oChan := make(chan []byte)

	// Create the orchestration-based client
	var oClient orchestration.Client
	switch *orch {
	case "kubernetes", "k8s", "openshift":
		var config *rest.Config
		if *inCluster {
			config, err = rest.InClusterConfig()
		} else {
			config, err = clientcmd.BuildConfigFromFlags("", *kubeConfig)
		}
		if err != nil {
			log.Fatalf("Error creating Kubernetes configuration: %v", err)
		}
		var kubeClient *kubernetes.Clientset
		kubeClient, err = kubernetes.NewForConfig(config)
		if err != nil {
			log.Fatalf("Error connecting to the Kubernetes client: %v", err)
		}
		params := orchestration.KubeParams{
			KubeClient:     kubeClient,
			Namespaces:     *namespaces,
			NamespaceLabel: *namespaceLabel,
			Channel:        oChan,
		}
		oClient, err = orchestration.NewKubernetesClient(&params)
	default:
		log.Fatalf("Unknown orchestration: %s", *orch)
	}
	if err != nil {
		log.Fatalf("%v", err)
	}
	// TODO: Set up the IPAM client
	ctlr := controller.NewController(oClient, oChan)
	ctlr.Run(stopCh)

	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)
	sig := <-signals
	log.Infof("Exiting - signal %v\n", sig)
	close(stopCh)
}
