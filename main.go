package main

import (
	"context"
	"fmt"

	"github.com/chanzuckerberg/go-misc/oidc_cli/v2/oidc_impl"

	"github.com/chanzuckerberg/go-misc/oidc_cli/v2/oidc_impl/client"

	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"strings"
)

type Recipient struct {
	Name string `json:"name"`
}

type RecipientsResponse struct {
	Recipients []Recipient `json:"recipients"`
}

func main() {
	clientID := "b8g7vhl1b312isgh5ik3gn6c"
	issuer := "https://cognito-idp.us-west-2.amazonaws.com/us-west-2_JLABROYbl"
	scopes := []string{"openid", "profile", "email"}
	clientInstance := &client.Client{
		OauthConfig: client.OauthConfig{},
	}

	// Apply scopes using SetScopeOptions
	setScopesOption := client.SetScopeOptions(scopes)
	setScopesOption(clientInstance)

	token, err := oidc_impl.GetToken(context.Background(), clientID, issuer)
	if err != nil {
		panic(err)
	}
	idToken := token.IDToken

	// Split the token into parts
	parts := strings.Split(idToken, ".")
	if len(parts) != 3 {
		fmt.Println("Invalid token format")
		return
	}

	// Decode the payload
	payloadBytes, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		fmt.Println("Error decoding payload:", err)
		return
	}
	var payload map[string]interface{}
	json.Unmarshal(payloadBytes, &payload)
	email := payload["email"].(string)
	username := strings.Split(email, "@")[0]

	// Replace with your Databricks workspace URL and token
	databricksURL := "https://czi-shared-infra-czi-sci-general-prod-databricks.cloud.databricks.com"
	pat := "<insert pat>"

	// The recipient name you want to check
	recipientName := username

	// Make the API request
	req, err := http.NewRequest("GET", fmt.Sprintf("%s/api/2.1/unity-catalog/recipients", databricksURL), nil)
	if err != nil {
		fmt.Println("Error creating request:", err)
		return
	}
	req.Header.Set("Authorization", "Bearer "+pat)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Println("Error making request:", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		fmt.Printf("Error: received status code %d\n", resp.StatusCode)
		body, _ := io.ReadAll(resp.Body)
		fmt.Println("Response body:", string(body))
		return
	}

	// Parse the response
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		fmt.Println("Error reading response:", err)
		return
	}

	var recipientsResponse RecipientsResponse
	err = json.Unmarshal(body, &recipientsResponse)
	if err != nil {
		fmt.Println("Error parsing JSON:", err)
		return
	}

	// Check if the recipient exists
	for _, recipient := range recipientsResponse.Recipients {
		if recipient.Name == recipientName {
			fmt.Printf("Recipient '%s' exists.\n", recipientName)
			return
		}
	}

	fmt.Printf("Recipient '%s' does not exist.\n", recipientName)
}
