package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/gofiber/fiber/v2"
)

var (
	databricksPAT     = os.Getenv("DATABRICKS_PAT")
	cognitoIssuer     = os.Getenv("COGNITO_ISSUER")
	databricksURL     = os.Getenv("DATABRICKS_URL")
	databricksAPIBase = fmt.Sprintf("%s/api/2.1/unity-catalog/recipients", databricksURL)
)

const expirationInSeconds = 604800 // 7 days

type TokenRequest struct {
	Token string `json:"token"`
}

type RecipientRequest struct {
	Name                string `json:"name"`
	AuthenticationType  string `json:"authentication_type"`
	TokenExpirationTime int    `json:"token_expiration_time"`
}

type RecipientResponse struct {
	Name               string         `json:"name"`
	AuthenticationType string         `json:"authentication_type"`
	Owner              string         `json:"owner"`
	CreatedAt          int64          `json:"created_at"`
	UpdatedAt          int64          `json:"updated_at"`
	FullName           string         `json:"full_name"`
	SecurableType      string         `json:"securable_type"`
	SecurableKind      string         `json:"securable_kind"`
	ID                 string         `json:"id"`
	Tokens             []TokenDetails `json:"tokens,omitempty"`
}

type TokenDetails struct {
	ID             string `json:"id"`
	ActivationURL  string `json:"activation_url"`
	ExpirationTime int64  `json:"expiration_time"`
	CreatedAt      int64  `json:"created_at"`
	CreatedBy      string `json:"created_by"`
	UpdatedAt      int64  `json:"updated_at"`
	UpdatedBy      string `json:"updated_by"`
}

type RecipientDetails struct {
	Name   string         `json:"name"`
	Tokens []TokenDetails `json:"tokens"`
}

type TokenRotationRequest struct {
	ExistingTokenExpireInSeconds int `json:"existing_token_expire_in_seconds"`
}

type TokenRotationResponse struct {
	Tokens []TokenDetails `json:"tokens"`
}

func validateCognitoToken(token string) (string, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return "", fmt.Errorf("invalid token format")
	}

	payloadBytes, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return "", fmt.Errorf("error decoding token payload: %w", err)
	}

	var payload map[string]interface{}
	if err := json.Unmarshal(payloadBytes, &payload); err != nil {
		return "", fmt.Errorf("error parsing token payload: %w", err)
	}

	if payload["iss"] != cognitoIssuer {
		return "", fmt.Errorf("token issuer is invalid")
	}

	email, ok := payload["email"].(string)
	if !ok {
		return "", fmt.Errorf("email not found in token")
	}

	return email, nil
}

func queryRecipient(email string) (*RecipientResponse, bool, error) {
	recipientName := strings.Split(email, "@")[0]
	url := fmt.Sprintf("%s/%s", databricksAPIBase, recipientName)

	resp, err := makeRequest("GET", url, nil)
	if err != nil {
		return nil, false, fmt.Errorf("error making request to Databricks: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, false, fmt.Errorf("error reading Databricks response body: %w", err)
	}

	// 🔹 Log full response for debugging
	fmt.Printf("Databricks Response (Status %d): %s\n", resp.StatusCode, string(body))

	if resp.StatusCode == http.StatusOK {
		var recipient RecipientResponse
		if err := json.Unmarshal(body, &recipient); err != nil {
			return nil, false, fmt.Errorf("error parsing recipient response JSON: %w", err)
		}

		// ✅ Check if `tokens` exists and has at least one entry
		hasTokens := len(recipient.Tokens) > 0

		return &recipient, hasTokens, nil
	} else if resp.StatusCode == http.StatusNotFound {
		return nil, false, nil // Recipient does not exist
	}

	return nil, false, fmt.Errorf("unexpected Databricks response: %d - %s", resp.StatusCode, string(body))
}

// Create a new recipient in Databricks
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
		return "", fmt.Errorf("error making request to create recipient: %w", err)
	}
	defer resp.Body.Close()

	// Read response body for debugging
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("error reading create-recipient response body: %w", err)
	}

	// 🔹 Log full response for debugging
	fmt.Printf("Databricks Create Recipient Response (Status %d): %s\n", resp.StatusCode, string(body))

	// ✅ Handle successful recipient creation
	if resp.StatusCode == http.StatusCreated {
		var recipientResponse RecipientResponse
		if err := json.Unmarshal(body, &recipientResponse); err != nil {
			return "", fmt.Errorf("error parsing create-recipient response JSON: %w", err)
		}

		fmt.Printf("Recipient '%s' created successfully. Activation link: %s\n", recipientName, recipientResponse.Tokens[0].ActivationURL)
		return recipientResponse.Tokens[0].ActivationURL, nil
	}

	// Handle unexpected responses
	return "", fmt.Errorf("failed to create recipient: %d - %s", resp.StatusCode, string(body))
}

// Rotate an expired token
func rotateToken(email string, expireInSeconds int) (string, error) {
	recipientName := strings.Split(email, "@")[0]
	url := fmt.Sprintf("%s/%s/rotate-token", databricksAPIBase, recipientName)

	// Prepare the request payload
	payload := TokenRotationRequest{
		ExistingTokenExpireInSeconds: expireInSeconds,
	}

	resp, err := makeRequest("POST", url, payload)
	if err != nil {
		return "", fmt.Errorf("error making request to rotate token: %w", err)
	}
	defer resp.Body.Close()

	// Read full response body for debugging
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("error reading Databricks rotate-token response body: %w", err)
	}

	// 🔹 Log full response for debugging
	fmt.Printf("Databricks Rotate Token Response (Status %d): %s\n", resp.StatusCode, string(body))

	// Check for successful response
	if resp.StatusCode == http.StatusOK {
		var rotationResponse TokenRotationResponse
		if err := json.Unmarshal(body, &rotationResponse); err != nil {
			return "", fmt.Errorf("error parsing rotate-token response JSON: %w", err)
		}

		// ✅ Ensure at least one token exists in the response
		if len(rotationResponse.Tokens) == 0 {
			return "", fmt.Errorf("rotate-token API returned no tokens")
		}

		// ✅ Extract the latest token (assuming last token in the list is the most recent)
		latestToken := rotationResponse.Tokens[len(rotationResponse.Tokens)-1]

		fmt.Printf("Token rotated successfully. New activation link: %s\n", latestToken.ActivationURL)
		return latestToken.ActivationURL, nil
	}

	// Handle unexpected responses
	return "", fmt.Errorf("failed to rotate token: %d - %s", resp.StatusCode, string(body))
}

// Send HTTP requests
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

	// Health Check
	app.Get("/health", func(c *fiber.Ctx) error {
		return c.Status(http.StatusOK).JSON(fiber.Map{"status": "healthy"})
	})

	// Token Verification & Databricks Recipient Handling
	app.Post("/verify-token", func(c *fiber.Ctx) error {
		var tokenRequest TokenRequest

		if err := c.BodyParser(&tokenRequest); err != nil {
			return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "Invalid request body"})
		}

		email, err := validateCognitoToken(tokenRequest.Token)
		if err != nil {
			return c.Status(http.StatusUnauthorized).JSON(fiber.Map{"error": "Invalid token: " + err.Error()})
		}

		// 🔹 Step 1: Check if recipient exists
		recipient, hasTokens, err := queryRecipient(email)
		if err != nil {
			return c.Status(http.StatusInternalServerError).JSON(fiber.Map{
				"error": "Error querying Databricks: " + err.Error(),
			})
		}

		// 🔹 Step 2: If recipient is missing, create it
		if recipient == nil {
			fmt.Printf("⚠️ Recipient for email '%s' does not exist. Creating...\n", email)
			activationLink, err := createRecipient(email)
			if err != nil {
				return c.Status(http.StatusInternalServerError).JSON(fiber.Map{
					"error": "Error creating recipient: " + err.Error(),
				})
			}
			return c.Status(http.StatusOK).JSON(fiber.Map{
				"message":         fmt.Sprintf("New recipient created for %s", email),
				"activation_link": activationLink,
			})
		}

		// 🔹 Step 3: If recipient exists but has no token, rotate a new token
		if !hasTokens {
			fmt.Printf("⚠️ Recipient '%s' exists but has no tokens. Rotating...\n", recipient.Name)
			activationLink, err := rotateToken(email, expirationInSeconds)
			if err != nil {
				return c.Status(http.StatusInternalServerError).JSON(fiber.Map{
					"error": "Error rotating token: " + err.Error(),
				})
			}
			return c.Status(http.StatusOK).JSON(fiber.Map{
				"message":         fmt.Sprintf("Token for %s rotated", email),
				"activation_link": activationLink,
			})
		}

		// 🔹 Step 4: If recipient exists and has a valid token, return it
		return c.Status(http.StatusOK).JSON(fiber.Map{
			"message":         fmt.Sprintf("Token for %s is still valid", email),
			"activation_link": recipient.Tokens[0].ActivationURL, // ✅ Safe to access
		})
	})

	log.Fatal(app.Listen(":8080"))
}
