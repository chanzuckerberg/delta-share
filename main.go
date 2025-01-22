package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"log/slog"
	"net/http"
	"os"
	"strings"

	"github.com/chanzuckerberg/go-misc/oidc_cli/oidc_impl"
	"github.com/chanzuckerberg/go-misc/oidc_cli/oidc_impl/client"
	"github.com/gofiber/fiber/v2"
)

type Recipient struct {
	Name string `json:"name"`
}

type RecipientsResponse struct {
	Recipients []Recipient `json:"recipients"`
}

var (
	// Set using argus set secrets
	databricksPAT    = os.Getenv("DATABRICKS_PAT")
	cognitoClientID  = os.Getenv("COGNITO_CLIENT_ID")
	cognitoIssuerURL = os.Getenv("COGNITO_ISSUER_URL")
)

func main() {
	if databricksPAT == "" {
		log.Panic("DATABRICKS_PAT cannot be blank")
	}

	if cognitoClientID == "" {
		log.Panic("COGNITO_CLIENT_ID cannot be blank")
	}

	if cognitoIssuerURL == "" {
		log.Panic("COGNITO_ISSUER_URL cannot be blank")
	}

	h := slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})
	slog.SetDefault(slog.New(h))

	app := fiber.New()

	app.Use(func(c *fiber.Ctx) error {
		if c.Path() != "/" && c.Path() != "/health" {
			logRequest(c)
		}
		return c.Next()
	})

	app.Get("/", healthHandler)
	app.Get("/health", healthHandler)

	scopes := []string{"openid", "profile", "email"}

	// Apply scopes using SetScopeOptions
	setScopesOption := client.SetScopeOptions(scopes)
	token, err := oidc_impl.GetToken(context.Background(), cognitoClientID, cognitoIssuerURL, setScopesOption)
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

	log.Fatal(app.Listen(":8080"))

	// Replace with your Databricks workspace URL and token
	databricksURL := "https://czi-shared-infra-czi-sci-general-prod-databricks.cloud.databricks.com"

	// The recipient name you want to check
	recipientName := username

	// Make the API request
	req, err := http.NewRequest("GET", fmt.Sprintf("%s/api/2.1/unity-catalog/recipients", databricksURL), nil)
	if err != nil {
		fmt.Println("Error creating request:", err)
		return
	}
	req.Header.Set("Authorization", "Bearer "+databricksPAT)

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

	fmt.Printf("Recipient '%s' does not exist. Please contact admin.\n", recipientName)
}

func healthHandler(c *fiber.Ctx) error {
	response := fiber.Map{"status": "healthy"}
	return c.JSON(response)
}
