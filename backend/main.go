package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
)

var (
	databricksPAT     = os.Getenv("DATABRICKS_PAT")
	cognitoIssuer     = os.Getenv("COGNITO_ISSUER")
	databricksURL     = os.Getenv("DATABRICKS_URL")
	databricksAPIBase = fmt.Sprintf("%s/api/2.1/unity-catalog/recipients", databricksURL)
)

const (
	expirationInSeconds = 604800 // 7 days in seconds
)

// TokenRequest represents the request body for verifying a token
type TokenRequest struct {
	Token string `json:"token"`
}

type RecipientRequest struct {
	Name                string `json:"name"`
	AuthenticationType  string `json:"authentication_type"`
	TokenExpirationTime int    `json:"token_expiration_time"`
}

type RecipientResponse struct {
	Name           string `json:"name"`
	ActivationLink string `json:"activation_link"`
	TokenInfo      struct {
		ActivationURL  string `json:"activation_url"`
		ExpirationTime int64  `json:"expiration_time"`
		Token          string `json:"token"`
	} `json:"token_info"`
}

type TokenRotationRequest struct {
	ExistingTokenExpireInSeconds int `json:"existing_token_expire_in_seconds"`
}

type TokenRotationResponse struct {
	Tokens []struct {
		ActivationURL  string `json:"activation_url"`
		ExpirationTime int64  `json:"expiration_time"`
	} `json:"tokens"`
}

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

// queryRecipient checks if a recipient exists and returns its token info.
func queryRecipient(email string) (*RecipientResponse, error) {
	recipientName := strings.Split(email, "@")[0]
	url := fmt.Sprintf("%s/%s", databricksAPIBase, recipientName)

	resp, err := makeRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode == http.StatusOK {
		var recipient RecipientResponse
		if err := json.NewDecoder(resp.Body).Decode(&recipient); err != nil {
			return nil, fmt.Errorf("error parsing recipient response: %w", err)
		}
		return &recipient, nil
	} else if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	} else {
		return nil, fmt.Errorf("unexpected response: %d", resp.StatusCode)
	}
}

// createRecipient creates a new recipient with token authentication and returns the token.
func createRecipient(email string) (string, error) {
	recipientName := strings.Split(email, "@")[0]
	url := databricksAPIBase

	payload := RecipientRequest{
		Name:                recipientName,
		AuthenticationType:  "TOKEN",
		TokenExpirationTime: expirationInSeconds,
	}

	resp, err := makeRequest("POST", url, payload)
	if err != nil {
		return "", err
	}

	if resp.StatusCode == http.StatusCreated {
		var recipient RecipientResponse
		if err := json.NewDecoder(resp.Body).Decode(&recipient); err != nil {
			return "", fmt.Errorf("error parsing recipient response: %w", err)
		}
		fmt.Printf("Recipient '%s' created successfully. Activation link: %s\n", recipientName, recipient.ActivationLink)
		return recipient.TokenInfo.Token, nil
	}

	return "", fmt.Errorf("failed to create recipient: %d", resp.StatusCode)
}

// rotateToken rotates the recipient’s token and returns the new token.
func rotateToken(email string, expireInSeconds int) (string, error) {
	recipientName := strings.Split(email, "@")[0]
	url := fmt.Sprintf("%s/%s/rotate-token", databricksAPIBase, recipientName)

	// Prepare the request body
	payload := TokenRotationRequest{
		ExistingTokenExpireInSeconds: expireInSeconds,
	}

	resp, err := makeRequest("POST", url, payload)

	if err != nil {
		return "", err
	}

	if resp.StatusCode == http.StatusOK {
		var rotationResponse TokenRotationResponse
		if err := json.NewDecoder(resp.Body).Decode(&rotationResponse); err != nil {
			return "", fmt.Errorf("error parsing token rotation response: %w", err)
		}
		fmt.Printf("Token rotated successfully. New activation link: %s\n", rotationResponse.Tokens[0].ActivationURL)
		return rotationResponse.Tokens[0].ActivationURL, nil
	}

	return "", fmt.Errorf("failed to rotate token: %d", resp.StatusCode)
}

// makeRequest is a helper function to send HTTP requests
func makeRequest(method, url string, payload interface{}) (*http.Response, error) {
	client := &http.Client{}

	var req *http.Request
	var err error

	if payload != nil {
		jsonData, _ := json.Marshal(payload)
		req, err = http.NewRequest(method, url, strings.NewReader(string(jsonData)))
	} else {
		req, err = http.NewRequest(method, url, nil)
	}

	if err != nil {
		return nil, fmt.Errorf("error creating request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+databricksPAT)
	req.Header.Set("Content-Type", "application/json")

	return client.Do(req)
}

func main() {
	app := fiber.New()

	// Health Check Endpoint
	app.Get("/health", func(c *fiber.Ctx) error {
		return c.Status(http.StatusOK).JSON(fiber.Map{
			"status": "healthy",
		})
	})

	// Token Verification & Databricks Recipient Handling
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
		recipient, err := queryRecipient(email)
		if err != nil {
			return c.Status(http.StatusInternalServerError).JSON(fiber.Map{
				"error": "Error querying Databricks: " + err.Error(),
			})
		}

		// If recipient does not exist, create one
		if recipient == nil {
			fmt.Printf("Recipient for email '%s' does not exist. Creating...\n", email)
			token, err := createRecipient(email)
			if err != nil {
				return c.Status(http.StatusInternalServerError).JSON(fiber.Map{
					"error": "Error creating recipient: " + err.Error(),
				})
			}
			return c.Status(http.StatusOK).JSON(fiber.Map{
				"message": fmt.Sprintf("New recipient created for %s", email),
				"token":   token,
			})
		}

		// Recipient exists but token is expired, so rotate it
		expirationTime := recipient.TokenInfo.ExpirationTime
		currentTime := time.Now().Unix()

		if expirationTime < currentTime {
			fmt.Printf("Token for recipient '%s' has expired. Rotating...\n", recipient.Name)
			activationLink, err := rotateToken(email, expirationInSeconds)
			if err != nil {
				return c.Status(http.StatusInternalServerError).JSON(fiber.Map{
					"error": "Error rotating token: " + err.Error(),
				})
			}
			return c.Status(http.StatusOK).JSON(fiber.Map{
				"message":         fmt.Sprintf("Token for %s rotated.", email),
				"activation_link": activationLink,
			})
		}

		// Recipient exists and token is still valid
		return c.Status(http.StatusOK).JSON(fiber.Map{
			"message":         fmt.Sprintf("Token for %s is still valid", email),
			"token":           recipient.TokenInfo.Token,
			"activation_link": recipient.TokenInfo.ActivationURL,
		})
	})

	// Start the Fiber app
	log.Fatal(app.Listen(":8080"))
}
