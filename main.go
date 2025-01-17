package main

import (
	"context"
	"fmt"

	"github.com/chanzuckerberg/go-misc/oidc_cli/v2/oidc_impl"

	"github.com/chanzuckerberg/go-misc/oidc_cli/v2/oidc_impl/client"

	"encoding/base64"
	"encoding/json"
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

	// Apply scopes using SetScopeOptions
	setScopesOption := client.SetScopeOptions(scopes)

	token, err := oidc_impl.GetToken(context.Background(), clientID, issuer, setScopesOption)
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

	// TODO: Call databricks server with username
}
