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

	var result map[string]interface{}
	err = json.Unmarshal(resp.Body(), &result)
	if err != nil {
		return "", err
	}

	auth := result["auth"].(map[string]interface{})
	return auth["client_token"].(string), nil
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

	var result map[string]interface{}
	err = json.Unmarshal(resp.Body(), &result)
	if err != nil {
		return "", err
	}

	plaintext, _ := base64.StdEncoding.DecodeString(result["data"].(map[string]interface{})["plaintext"].(string))
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

	var result map[string]interface{}
	err = json.Unmarshal(resp.Body(), &result)
	if err != nil {
		return "", err
	}

	data := result["data"].(map[string]interface{})["data"].(map[string]interface{})
	return data[secretKey].(string), nil
}
