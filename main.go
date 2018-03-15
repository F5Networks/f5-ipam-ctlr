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
	"io/ioutil"
	"os"
	"os/signal"
	"strings"
	"syscall"

	log "github.com/F5Networks/f5-ipam-ctlr/pkg/vlogger"
	clog "github.com/F5Networks/f5-ipam-ctlr/pkg/vlogger/console"
	flag "github.com/spf13/pflag"
	"golang.org/x/crypto/ssh/terminal"

	"github.com/F5Networks/f5-ipam-ctlr/pkg/controller"
	"github.com/F5Networks/f5-ipam-ctlr/pkg/manager"
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
	globalFlags *flag.FlagSet
	kubeFlags   *flag.FlagSet
	ibFlags     *flag.FlagSet

	// Global
	logLevel       *string
	orch           *string
	mgr            *string
	verifyInterval *int
	printVersion   *bool

	// Kubernetes
	inCluster      *bool
	kubeConfig     *string
	namespaces     *[]string
	namespaceLabel *string

	// Infoblox
	ibHost     *string
	ibVersion  *string
	ibPort     *int
	ibUsername *string
	ibPassword *string
	credsDir   *string
)

const INFOBLOX = "infoblox"

func init() {
	flags = flag.NewFlagSet("main", flag.ContinueOnError)
	globalFlags = flag.NewFlagSet("Global", flag.ContinueOnError)
	kubeFlags = flag.NewFlagSet("Kubernetes", flag.ContinueOnError)
	ibFlags = flag.NewFlagSet("Infoblox", flag.ContinueOnError)

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
	mgr = globalFlags.String("ip-manager", "",
		"Required, the IPAM system that the controller will interface with.")
	verifyInterval = globalFlags.Int("verify-interval", 30,
		"Optional, interval (in seconds) at which to verify the IPAM system configuration. "+
			"Set to 0 to disable.")
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

	// Infoblox flags
	ibHost = ibFlags.String("infoblox-grid-host", "",
		"Required for infoblox, the grid manager host IP.")
	ibVersion = ibFlags.String("infoblox-wapi-version", "",
		"Required for infoblox, the Web API version.")
	ibPort = ibFlags.Int("infoblox-wapi-port", 443,
		"Optional for infoblox, the Web API port.")
	ibUsername = ibFlags.String("infoblox-username", "",
		"Required for infoblox, the login username.")
	ibPassword = ibFlags.String("infoblox-password", "",
		"Required for infoblox, the login password.")
	credsDir = ibFlags.String("credentials-directory", "",
		"Optional for infoblox, directory that contains the infoblox username and "+
			"password files. To be used instead of username and password arguments.")

	globalFlags.Usage = func() {
		fmt.Fprintf(os.Stderr, "  Global:\n%s\n", globalFlags.FlagUsagesWrapped(width))
	}
	kubeFlags.Usage = func() {
		fmt.Fprintf(os.Stderr, "  Kubernetes:\n%s\n", kubeFlags.FlagUsagesWrapped(width))
	}
	ibFlags.Usage = func() {
		fmt.Fprintf(os.Stderr, "  Infoblox:\n%s\n", ibFlags.FlagUsagesWrapped(width))
	}
	flags.AddFlagSet(globalFlags)
	flags.AddFlagSet(kubeFlags)
	flags.AddFlagSet(ibFlags)

	flags.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage of %s\n", os.Args[0])
		globalFlags.Usage()
		kubeFlags.Usage()
		ibFlags.Usage()
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

	if len(*mgr) == 0 {
		return fmt.Errorf("IP-Manager is required.")
	}

	if *verifyInterval < 0 {
		return fmt.Errorf("Cannot use negative verify interval.")
	}

	*orch = strings.ToLower(*orch)
	*mgr = strings.ToLower(*mgr)

	if len(*namespaces) != 0 && len(*namespaceLabel) != 0 {
		return fmt.Errorf("Cannot specify both namespace and namespace-label.")
	}

	if *mgr == INFOBLOX {
		if len(*ibHost) == 0 || len(*ibVersion) == 0 {
			return fmt.Errorf("Missing required Infoblox parameter.")
		} else if (len(*ibUsername) == 0 || len(*ibPassword) == 0) && len(*credsDir) == 0 {
			return fmt.Errorf("Missing Infoblox credentials.")
		} else if len(*ibUsername) > 0 && len(*ibPassword) > 0 && len(*credsDir) > 0 {
			return fmt.Errorf(
				"Please specify either credentials directory OR username/password, not both.")
		}
	}

	return nil
}

func getCredentials() {
	if *mgr == INFOBLOX && len(*credsDir) > 0 {
		var usr, pass string
		var usrBytes, passBytes []byte
		var err error
		if strings.HasSuffix(*credsDir, "/") {
			usr = *credsDir + "infoblox-username"
			pass = *credsDir + "infoblox-password"
		} else {
			usr = *credsDir + "/infoblox-username"
			pass = *credsDir + "/infoblox-password"
		}

		usrBytes, err = ioutil.ReadFile(usr)
		if err != nil {
			log.Fatalf("%v", err)
		}
		*ibUsername = string(usrBytes)

		passBytes, err = ioutil.ReadFile(pass)
		if err != nil {
			log.Fatalf("%v", err)
		}
		*ibPassword = string(passBytes)
	}
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
	getCredentials()

	log.Infof("Starting: Version: %s, BuildInfo: %s", version, buildInfo)

	stopCh := make(chan struct{})

	// Create a channel for the orchestration client to send data to the controller
	oChan := make(chan *orchestration.IPGroup)

	orchestration.IPAM = *mgr

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

	// Create the IPAM system client
	var iClient manager.Client
	switch *mgr {
	case INFOBLOX:
		params := manager.InfobloxParams{
			Host:     *ibHost,
			Version:  *ibVersion,
			Port:     *ibPort,
			Username: *ibUsername,
			Password: *ibPassword,
		}
		iClient, err = manager.NewInfobloxClient(&params, *orch)
	default:
		log.Fatalf("Unknown IP manager: %s", *mgr)
	}
	if err != nil {
		log.Fatalf("%v", err)
	}

	ctlr := controller.NewController(oClient, iClient, oChan, *verifyInterval)
	ctlr.Run(stopCh)

	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)
	sig := <-signals
	log.Infof("Exiting - signal %v\n", sig)
	close(stopCh)
}
