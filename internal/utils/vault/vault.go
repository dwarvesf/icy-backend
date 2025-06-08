package vault

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"

	"github.com/go-resty/resty/v2"
)

// VaultClient represents the Vault interaction client
type VaultClient struct {
	addr         string
	kvSecretPath string
	role         string
	token        string
}

// New creates a new Vault client
func New(addr, kvSecretPath, role string) *VaultClient {
	vc := &VaultClient{
		addr:         addr,
		role:         role,
		kvSecretPath: kvSecretPath,
	}
	var err error
	vc.token, err = vc.login()
	if err != nil {
		panic(err)
	}
	return vc
}

// GetKubernetesToken reads the Kubernetes service account token
func (vc *VaultClient) GetKubernetesToken() (string, error) {
	token, err := os.ReadFile("/var/run/secrets/kubernetes.io/serviceaccount/token")
	if err != nil {
		return "", fmt.Errorf("failed to read token: %v", err)
	}
	return string(token), nil
}

// Login performs login to Vault using Kubernetes authentication
func (vc *VaultClient) login() (string, error) {
	k8sToken, err := vc.GetKubernetesToken()
	if err != nil {
		return "", err
	}

	client := resty.New()
	resp, err := client.R().
		SetHeader("Content-Type", "application/json").
		SetBody(map[string]string{
			"jwt":  k8sToken,
			"role": vc.role,
		}).
		Post(fmt.Sprintf("%s/v1/auth/kubernetes/login", vc.addr))

	if err != nil {
		return "", err
	}

	// Check HTTP status code
	if resp.StatusCode() != 200 {
		return "", fmt.Errorf("vault authentication failed with status %d: %s", resp.StatusCode(), string(resp.Body()))
	}

	var result map[string]interface{}
	err = json.Unmarshal(resp.Body(), &result)
	if err != nil {
		return "", fmt.Errorf("failed to parse vault response: %v", err)
	}

	// Check if there's an error in the response
	if errMsg, exists := result["errors"]; exists {
		return "", fmt.Errorf("vault authentication error: %v", errMsg)
	}

	// Check if auth field exists and is not nil
	authInterface, exists := result["auth"]
	if !exists {
		return "", fmt.Errorf("vault response missing 'auth' field")
	}

	if authInterface == nil {
		return "", fmt.Errorf("vault 'auth' field is nil")
	}

	// Type assert to map[string]interface{}
	auth, ok := authInterface.(map[string]interface{})
	if !ok {
		return "", fmt.Errorf("vault 'auth' field is not a valid object")
	}

	// Check if client_token exists
	tokenInterface, exists := auth["client_token"]
	if !exists {
		return "", fmt.Errorf("vault response missing 'client_token' field")
	}

	// Type assert to string
	token, ok := tokenInterface.(string)
	if !ok {
		return "", fmt.Errorf("vault 'client_token' is not a string")
	}

	if token == "" {
		return "", fmt.Errorf("vault returned empty client_token")
	}

	return token, nil
}

// DecryptData decrypts data using Vault's transit engine
func (vc *VaultClient) DecryptData(transitKey, ciphertext string) (string, error) {
	client := resty.New()
	resp, err := client.R().
		SetHeader("X-Vault-Token", vc.token).
		SetBody(map[string]string{"ciphertext": ciphertext}).
		Post(fmt.Sprintf("%s/v1/transit/decrypt/%s", vc.addr, transitKey))

	if err != nil {
		return "", err
	}

	// Check HTTP status code
	if resp.StatusCode() != 200 {
		return "", fmt.Errorf("vault decrypt failed with status %d: %s", resp.StatusCode(), string(resp.Body()))
	}

	var result map[string]interface{}
	err = json.Unmarshal(resp.Body(), &result)
	if err != nil {
		return "", fmt.Errorf("failed to parse vault response: %v", err)
	}

	// Check if there's an error in the response
	if errMsg, exists := result["errors"]; exists {
		return "", fmt.Errorf("vault decrypt error: %v", errMsg)
	}

	// Check if data field exists
	dataInterface, exists := result["data"]
	if !exists {
		return "", fmt.Errorf("vault response missing 'data' field")
	}

	if dataInterface == nil {
		return "", fmt.Errorf("vault 'data' field is nil")
	}

	// Type assert to map[string]interface{}
	data, ok := dataInterface.(map[string]interface{})
	if !ok {
		return "", fmt.Errorf("vault 'data' field is not a valid object")
	}

	// Check if plaintext field exists
	plaintextInterface, exists := data["plaintext"]
	if !exists {
		return "", fmt.Errorf("vault response missing 'plaintext' field")
	}

	// Type assert to string
	plaintextB64, ok := plaintextInterface.(string)
	if !ok {
		return "", fmt.Errorf("vault 'plaintext' is not a string")
	}

	plaintext, err := base64.StdEncoding.DecodeString(plaintextB64)
	if err != nil {
		return "", fmt.Errorf("failed to decode base64 plaintext: %v", err)
	}

	return string(plaintext), nil
}

// GetKV retrieves a secret from Vault's Key-Value store
func (vc *VaultClient) GetKV(secretKey string) (string, error) {
	client := resty.New()
	resp, err := client.R().
		SetHeader("X-Vault-Token", vc.token).
		Get(fmt.Sprintf("%s/v1/%s", vc.addr, vc.kvSecretPath))

	if err != nil {
		return "", err
	}

	// Check HTTP status code
	if resp.StatusCode() != 200 {
		return "", fmt.Errorf("vault KV get failed with status %d: %s", resp.StatusCode(), string(resp.Body()))
	}

	var result map[string]interface{}
	err = json.Unmarshal(resp.Body(), &result)
	if err != nil {
		return "", fmt.Errorf("failed to parse vault response: %v", err)
	}

	// Check if there's an error in the response
	if errMsg, exists := result["errors"]; exists {
		return "", fmt.Errorf("vault KV get error: %v", errMsg)
	}

	// Check if data field exists
	dataInterface, exists := result["data"]
	if !exists {
		return "", fmt.Errorf("vault response missing 'data' field")
	}

	if dataInterface == nil {
		return "", fmt.Errorf("vault 'data' field is nil")
	}

	// Type assert to map[string]interface{}
	data, ok := dataInterface.(map[string]interface{})
	if !ok {
		return "", fmt.Errorf("vault 'data' field is not a valid object")
	}

	// Check if nested data field exists (for KV v2)
	nestedDataInterface, exists := data["data"]
	if !exists {
		return "", fmt.Errorf("vault response missing nested 'data' field")
	}

	if nestedDataInterface == nil {
		return "", fmt.Errorf("vault nested 'data' field is nil")
	}

	// Type assert to map[string]interface{}
	nestedData, ok := nestedDataInterface.(map[string]interface{})
	if !ok {
		return "", fmt.Errorf("vault nested 'data' field is not a valid object")
	}

	// Check if the requested secret key exists
	secretInterface, exists := nestedData[secretKey]
	if !exists {
		return "", fmt.Errorf("secret key '%s' not found", secretKey)
	}

	// Type assert to string
	secret, ok := secretInterface.(string)
	if !ok {
		return "", fmt.Errorf("secret value for key '%s' is not a string", secretKey)
	}

	return secret, nil
}
