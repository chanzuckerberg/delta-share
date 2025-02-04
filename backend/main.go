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
	Recipients []RecipientDetails `json:"recipients"`
}

type RecipientDetails struct {
	Name   string         `json:"name"`
	Tokens []TokenDetails `json:"tokens"`
}

type TokenDetails struct {
	ActivationURL  string `json:"activation_url"`
	ExpirationTime int64  `json:"expiration_time"`
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

// Query Databricks API for recipient info
func queryRecipient(email string) (*RecipientResponse, error) {
	recipientName := strings.Split(email, "@")[0]
	url := fmt.Sprintf("%s/%s", databricksAPIBase, recipientName)

	resp, err := makeRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode == http.StatusOK {
		var recipient RecipientResponse
		fmt.Printf("Recipient response body: %+v\n", resp.Body)
		fmt.Print(json.NewDecoder(resp.Body).Decode(&recipient))
		if err := json.NewDecoder(resp.Body).Decode(&recipient); err != nil {
			return nil, fmt.Errorf("error parsing recipient response: %w", err)
		}
		return &recipient, nil
	} else if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}

	return nil, fmt.Errorf("unexpected response: %d", resp.StatusCode)
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
		return "", err
	}

	if resp.StatusCode == http.StatusCreated {
		var recipient RecipientResponse
		if err := json.NewDecoder(resp.Body).Decode(&recipient); err != nil {
			return "", fmt.Errorf("error parsing recipient response: %w", err)
		}

		if len(recipient.Recipients) == 0 {
			return "", fmt.Errorf("no tokens returned for new recipient")
		}

		// fmt.Printf("Recipient '%s' created successfully. Activation link: %s\n", recipientName, recipient.Recipients[0].Tokens[0].ActivationURL)
		// return recipient.Recipients[0].Tokens[0].ActivationURL, nil
		return "trying to send activation link", nil
	}

	return "", fmt.Errorf("failed to create recipient: %d", resp.StatusCode)
}

// Rotate an expired token
func rotateToken(email string, expireInSeconds int) (string, error) {
	recipientName := strings.Split(email, "@")[0]
	url := fmt.Sprintf("%s/%s/rotate-token", databricksAPIBase, recipientName)

	payload := TokenRotationRequest{ExistingTokenExpireInSeconds: expireInSeconds}
	resp, err := makeRequest("POST", url, payload)
	if err != nil {
		return "", err
	}

	if resp.StatusCode == http.StatusOK {
		var rotationResponse TokenRotationResponse
		if err := json.NewDecoder(resp.Body).Decode(&rotationResponse); err != nil {
			return "", fmt.Errorf("error parsing token rotation response: %w", err)
		}

		if len(rotationResponse.Tokens) == 0 {
			return "", fmt.Errorf("no tokens returned after rotation")
		}

		// fmt.Printf("Token rotated successfully. New activation link: %s\n", rotationResponse.Tokens[0].ActivationURL)
		// return rotationResponse.Tokens[0].ActivationURL, nil
		return "trying to send activation link", nil
	}

	return "", fmt.Errorf("failed to rotate token: %d", resp.StatusCode)
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

		recipient, err := queryRecipient(email)
		if err != nil {
			return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": "Error querying Databricks: " + err.Error()})
		}

		// Create recipient if not found
		if recipient == nil {
			fmt.Printf("Recipient for email '%s' does not exist. Creating...\n", email)
			token, err := createRecipient(email)
			if err != nil {
				return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": "Error creating recipient: " + err.Error()})
			}
			return c.Status(http.StatusOK).JSON(fiber.Map{"message": fmt.Sprintf("New recipient created for %s", email), "activation_link": token})
		}

		// Check token expiration
		fmt.Printf("Recipient response: %+v\n", recipient)
		// expirationTime := recipient.Recipients[0].Tokens[0].ExpirationTime
		// currentTime := time.Now().Unix()

		// if expirationTime < currentTime {
		// 	fmt.Printf("Token for recipient '%s' has expired. Rotating...\n", recipient.Recipients[0].Name)
		// 	activationLink, err := rotateToken(email, expirationInSeconds)
		// 	if err != nil {
		// 		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": "Error rotating token: " + err.Error()})
		// 	}
		// 	return c.Status(http.StatusOK).JSON(fiber.Map{"message": fmt.Sprintf("Token for %s rotated", email), "activation_link": activationLink})
		// }

		// return c.Status(http.StatusOK).JSON(fiber.Map{"message": fmt.Sprintf("Token for %s is still valid", email), "activation_link": recipient.Recipients[0].Tokens[0].ActivationURL})
	})

	log.Fatal(app.Listen(":8080"))
}
