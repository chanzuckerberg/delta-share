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
	TokenExpirationTime int    `json:"expiration_time"`
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

	fmt.Printf("🔍 Raw Databricks API Response: %s\n", string(body))

	if resp.StatusCode == http.StatusOK {
		var recipient RecipientResponse
		if err := json.Unmarshal(body, &recipient); err != nil {
			return nil, false, fmt.Errorf("error parsing recipient response JSON: %w", err)
		}

		hasTokens := len(recipient.Tokens) > 0

		return &recipient, hasTokens, nil
	} else if resp.StatusCode == http.StatusNotFound {
		return nil, false, nil // Recipient does not exist
	}

	return nil, false, fmt.Errorf("unexpected Databricks response: %d - %s", resp.StatusCode, string(body))
}

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

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("error reading create-recipient response body: %w", err)
	}

	if resp.StatusCode == http.StatusOK {
		var recipientResponse RecipientResponse
		if err := json.Unmarshal(body, &recipientResponse); err != nil {
			return "", fmt.Errorf("error parsing create-recipient response JSON: %w", err)
		}

		fmt.Printf("Recipient '%s' created successfully. Activation link: %s\n", recipientName, recipientResponse.Tokens[0].ActivationURL)
		return recipientResponse.Tokens[0].ActivationURL, nil
	}

	return "", fmt.Errorf("failed to create recipient: %d - %s", resp.StatusCode, string(body))
}

func rotateToken(email string, expireInSeconds int) (string, error) {
	recipientName := strings.Split(email, "@")[0]
	url := fmt.Sprintf("%s/%s/rotate-token", databricksAPIBase, recipientName)

	payload := TokenRotationRequest{
		ExistingTokenExpireInSeconds: expireInSeconds,
	}

	resp, err := makeRequest("POST", url, payload)
	if err != nil {
		return "", fmt.Errorf("error making request to rotate token: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("error reading Databricks rotate-token response body: %w", err)
	}

	if resp.StatusCode == http.StatusOK {
		var rotationResponse TokenRotationResponse
		if err := json.Unmarshal(body, &rotationResponse); err != nil {
			return "", fmt.Errorf("error parsing rotate-token response JSON: %w", err)
		}

		token := rotationResponse.Tokens[0]

		fmt.Printf("Token rotated successfully. New activation link: %s\n", token.ActivationURL)
		return token.ActivationURL, nil
	}

	return "", fmt.Errorf("failed to rotate token: %d - %s", resp.StatusCode, string(body))
}

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

	app.Get("/health", func(c *fiber.Ctx) error {
		return c.Status(http.StatusOK).JSON(fiber.Map{"status": "healthy"})
	})

	app.Post("/verify-token", func(c *fiber.Ctx) error {
		var tokenRequest TokenRequest

		if err := c.BodyParser(&tokenRequest); err != nil {
			return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "Invalid request body"})
		}

		email, err := validateCognitoToken(tokenRequest.Token)
		if err != nil {
			return c.Status(http.StatusUnauthorized).JSON(fiber.Map{"error": "Invalid token: " + err.Error()})
		}

		// Check if recipient exists
		recipient, hasTokens, err := queryRecipient(email)
		if err != nil {
			return c.Status(http.StatusInternalServerError).JSON(fiber.Map{
				"error": "Error querying Databricks: " + err.Error(),
			})
		}

		// If recipient is missing, create it
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

		// If recipient exists but has no token, rotate a new token
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

		// If recipient exists and has a valid token, return it
		fmt.Printf("🔍 Debug: Recipient struct: %+v\n", recipient)
		return c.Status(http.StatusOK).JSON(fiber.Map{
			"message":         fmt.Sprintf("Token for %s is still valid", email),
			"activation_link": recipient.Tokens[0].ActivationURL,
		})
	})

	log.Fatal(app.Listen(":8080"))
}
