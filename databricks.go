package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"log/slog"
	"net/http"
	"os"

	"github.com/gofiber/fiber"
)

var (
	databricksPAT          = os.Getenv("DATABRICKS_PAT")
	databricksWorkspaceURL = os.Getenv("DATABRICKS_URL")
)

func runDatabricks(username string) {
	if databricksPAT == "" {
		log.Panic("DATABRICKS_PAT cannot be blank")
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

	log.Fatal(app.Listen(":8080"))

	// The recipient name you want to check
	recipientName := username

	// Make the API request
	req, err := http.NewRequest("GET", fmt.Sprintf("%s/api/2.1/unity-catalog/recipients", databricksWorkspaceURL), nil)
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

	fmt.Printf("Recipient '%s' does not exist.\n", recipientName)
}

func healthHandler(c *fiber.Ctx) error {
	response := fiber.Map{"status": "healthy"}
	return c.JSON(response)
}
