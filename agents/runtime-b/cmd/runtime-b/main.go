package main

import (
	"log"
	"net/http"
	"os"

	"github.com/Nene7ko/NeKiro/agents/internal/challengeproof"
	runtimeb "github.com/Nene7ko/NeKiro/agents/runtime-b"
	"github.com/Nene7ko/NeKiro/sdks/agent-sdk/routerauth"
)

func main() {
	address, err := runtimeb.ListenAddressFromEnvironment(os.LookupEnv)
	if err != nil {
		log.Fatal(err)
	}
	authenticationConfig, err := routerauth.LoadConfig(os.LookupEnv)
	if err != nil {
		log.Fatal("runtime-b authentication config: ", err)
	}
	execution, err := runtimeb.NewHTTPHandlerWithAuth(runtimeb.NewHandler(), authenticationConfig)
	if err != nil {
		log.Fatal("runtime-b authentication: ", err)
	}
	application, err := challengeproof.NewHandler(execution, os.LookupEnv)
	if err != nil {
		log.Fatal("runtime-b challenge proof: ", err)
	}
	if err := http.ListenAndServe(address, application); err != nil {
		log.Fatal(err)
	}
}
