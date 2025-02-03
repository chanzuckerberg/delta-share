package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/gofiber/fiber/v2"
)

// Structs for request/response
type TokenRequest struct {
	Token string `json:"token"`
}

type Recipient struct {
	Name string `json:"name"`
}
type RecipientsResponse struct {
	Recipients []Recipient `json:"recipients"`
}

// Environment Variables stored using argus set secrets
var (
	databricksPAT = os.Getenv("DATABRICKS_PAT")
	databricksURL = os.Getenv("DATABRICKS_URL")
	cognitoIssuer = os.Getenv("COGNITO_ISSUER")
)

func main() {
	app := fiber.New()

	// Health Check Endpoint
	app.Get("/health", func(c *fiber.Ctx) error {
		return c.Status(http.StatusOK).JSON(fiber.Map{
			"status": "healthy",
		})
	})

	// Token Verification Endpoint
	app.Post("/verify-token", func(c *fiber.Ctx) error {
		var tokenRequest TokenRequest

		// Parse the incoming JSON body
		if err := c.BodyParser(&tokenRequest); err != nil {
			return c.Status(http.StatusBadRequest).JSON(fiber.Map{
				"error": "Invalid request body",
			})
		}

		// Decode and Validate the Cognito Token
		email, err := validateCognitoToken(tokenRequest.Token)
		if err != nil {
			return c.Status(http.StatusUnauthorized).JSON(fiber.Map{
				"error": "Invalid token: " + err.Error(),
			})
		}

		// Query Databricks API for Delta Share recipient
		isRecipient, err := queryDatabricksForRecipient(email)
		if err != nil {
			return c.Status(http.StatusInternalServerError).JSON(fiber.Map{
				"error": "Error querying Databricks: " + err.Error(),
			})
		}

		if !isRecipient {
			return c.Status(http.StatusForbidden).JSON(fiber.Map{
				"error": fmt.Sprintf("User %s is not a Delta Share recipient", email),
			})
		}

		// Return Success Response
		return c.Status(http.StatusOK).JSON(fiber.Map{
			"message":        fmt.Sprintf("User %s is a Delta Share recipient", email),
			"databricks_url": databricksURL,
		})
	})

	// Start the Fiber app
	log.Fatal(app.Listen(":8080"))
}

// Validate and Decode Cognito Token
func validateCognitoToken(token string) (string, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return "", fmt.Errorf("invalid token format")
	}

	// Decode the payload (middle part of the JWT)
	payloadBytes, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return "", fmt.Errorf("error decoding token payload: %w", err)
	}

	// Parse the payload as JSON
	var payload map[string]interface{}
	if err := json.Unmarshal(payloadBytes, &payload); err != nil {
		return "", fmt.Errorf("error parsing token payload: %w", err)
	}

	// Ensure the token is issued by your Cognito issuer
	if payload["iss"] != cognitoIssuer {
		return "", fmt.Errorf("token issuer is invalid")
	}

	// Extract and return the email
	email, ok := payload["email"].(string)
	if !ok {
		return "", fmt.Errorf("email not found in token")
	}

	return email, nil
}

// queryDatabricksForRecipient checks if a recipient exists based on the provided email.
func queryDatabricksForRecipient(email string) (bool, error) {
	url := fmt.Sprintf("%s/%s", databricksURL+"/api/2.1/unity-catalog/recipients", email)

	// Make the API request
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return false, fmt.Errorf("error creating request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+databricksPAT)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return false, fmt.Errorf("error making request: %w", err)
	}
	defer resp.Body.Close()

	// Handle HTTP response status codes
	if resp.StatusCode == http.StatusOK {
		return true, nil
	} else if resp.StatusCode == http.StatusNotFound {
		return false, nil
	} else {
		return false, fmt.Errorf("unexpected response: %d", resp.StatusCode)
	}
}
