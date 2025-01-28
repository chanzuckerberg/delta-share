package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/chanzuckerberg/go-misc/oidc_cli/oidc_impl"
	"github.com/chanzuckerberg/go-misc/oidc_cli/oidc_impl/client"
)

func main() {
	scopes := []string{"openid", "profile", "email"}
	cognitoClientID := "4p1qrneiifjc4npqgoikblc1kv"
	cognitoIssuerURL := "https://cognito-idp.us-west-2.amazonaws.com/us-west-2_kQfwBKR2t"

	// Get the Cognito Token
	setScopesOption := client.SetScopeOptions(scopes)
	token, err := oidc_impl.GetToken(context.Background(), cognitoClientID, cognitoIssuerURL, setScopesOption)
	if err != nil {
		panic(err)
	}

	// Send the token to the backend
	// TODO: Replace with your backend URL I think it'd be https://delta-share.prod-sci-general.prod.czi.team
	backendURL := "https://delta-share.prod-sci-general.prod.czi.team/verify-token"
	reqBody := map[string]string{"token": token.IDToken}
	reqBodyJSON, _ := json.Marshal(reqBody)

	resp, err := http.Post(backendURL, "application/json", strings.NewReader(string(reqBodyJSON)))
	if err != nil {
		fmt.Println("Error communicating with backend:", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		fmt.Printf("Error from backend: %s\n", body)
		return
	}

	fmt.Println("User successfully verified!")
}
