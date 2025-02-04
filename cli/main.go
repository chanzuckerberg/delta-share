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
	// backendURL := "https://delta-share.prod-sci-general.prod.czi.team/verify-token"
	backendURL := "https://electric-osprey.dev-sci-general.dev.czi.team/verify-token"
	reqBody := map[string]string{"token": token.IDToken}
	reqBodyJSON, _ := json.Marshal(reqBody)

	resp, err := http.Post(backendURL, "application/json", strings.NewReader(string(reqBodyJSON)))
	if err != nil {
		fmt.Println("Error communicating with backend:", err)
		return
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		fmt.Println("Error reading response:", err)
		return
	}

	// Handle non-200 responses
	if resp.StatusCode != http.StatusOK {
		fmt.Printf("Error from backend (Status %d): %s\n", resp.StatusCode, string(body))
		return
	}

	// Parse JSON response
	var response struct {
		Message        string `json:"message"`
		ActivationLink string `json:"activation_link"`
	}

	if err := json.Unmarshal(body, &response); err != nil {
		fmt.Println("Error parsing response:", err)
		return
	}

	// Print the success message and activation link
	fmt.Println(response.Message)
	if response.ActivationLink != "" {
		fmt.Println("Activation Link:", response.ActivationLink)
	}
}
