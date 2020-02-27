package keyvault

import (
	"context"
	"fmt"

	"encoding/json"

	mgmtclient "github.com/Azure/azure-sdk-for-go/services/keyvault/mgmt/2018-02-14/keyvault"
	"github.com/Azure/azure-sdk-for-go/services/keyvault/v7.0/keyvault"
	keyvaults "github.com/Azure/azure-sdk-for-go/services/keyvault/v7.0/keyvault"
	"github.com/Azure/azure-service-operator/api/v1alpha1"
	"github.com/Azure/azure-service-operator/pkg/resourcemanager/config"
	"github.com/Azure/azure-service-operator/pkg/resourcemanager/iam"
	"github.com/Azure/azure-service-operator/pkg/secrets"
	"github.com/Azure/go-autorest/autorest/date"
	"k8s.io/apimachinery/pkg/runtime"
	"github.com/Azure/go-autorest/autorest/to"
	"k8s.io/apimachinery/pkg/types"
)

func getVaultsClient() (mgmtclient.VaultsClient, error) {
	vaultsClient := mgmtclient.NewVaultsClient(config.SubscriptionID())
	a, err := iam.GetResourceManagementAuthorizer()
	if err != nil {
		return vaultsClient, err
	}
	vaultsClient.Authorizer = a
	vaultsClient.AddToUserAgent(config.UserAgent())
	return vaultsClient, nil
}

func GetVault(ctx context.Context, groupName string, vaultName string) (result mgmtclient.Vault, err error) {
	vaultsClient, err := getVaultsClient()
	if err != nil {
		return mgmtclient.Vault{}, err
	}
	return vaultsClient.Get(ctx, groupName, vaultName)

}

// KeyvaultSecretClient struct has the Key vault BaseClient that Azure uses and the KeyVault name
type KeyvaultSecretClient struct {
	KeyVaultClient keyvaults.BaseClient
	KeyVaultName   string
}

// GetKeyVaultName extracts the KeyVault name from the generic runtime object
func GetKeyVaultName(instance runtime.Object) string {
	keyVaultName := ""
	target := &v1alpha1.GenericResource{}
	serial, err := json.Marshal(instance)
	if err != nil {
		return keyVaultName
	}
	_ = json.Unmarshal(serial, target)
	return target.Spec.KeyVaultToStoreSecrets
}

func getVaultsURL(ctx context.Context, vaultName string) string {
	vaultURL := "https://" + vaultName + ".vault.azure.net" //default
	vault, err := GetVault(ctx, "", vaultName)
	if err == nil {
		vaultURL = *vault.Properties.VaultURI
	}
	return vaultURL
}

// New instantiates a new KeyVaultSecretClient instance
func New(keyvaultName string) *KeyvaultSecretClient {
	keyvaultClient := keyvaults.New()
	a, _ := iam.GetKeyvaultAuthorizer()
	keyvaultClient.Authorizer = a
	keyvaultClient.AddToUserAgent(config.UserAgent())
	return &KeyvaultSecretClient{
		KeyVaultClient: keyvaultClient,
		KeyVaultName:   keyvaultName,
	}
}

// Create creates a key in KeyVault if it does not exist already
func (k *KeyvaultSecretClient) Create(ctx context.Context, key types.NamespacedName, data map[string][]byte, opts ...secrets.SecretOption) error {
	options := &secrets.Options{}
	for _, opt := range opts {
		opt(options)
	}

	var secretName string
	vaultBaseURL := getVaultsURL(ctx, k.KeyVaultName)
	if len(key.Namespace) != 0 {
		secretName = key.Namespace + "-" + key.Name
	} else {
		secretName = key.Name
	}
	secretVersion := ""
	enabled := true
	var activationDateUTC date.UnixTime
	var expireDateUTC date.UnixTime

	// Convert the map into a string as that's what a KeyVault secret takes
	jsonData, err := json.Marshal(data)
	if err != nil {
		return err
	}
	stringSecret := string(jsonData)

	// Initialize secret attributes
	secretAttributes := keyvaults.SecretAttributes{
		Enabled: &enabled,
	}

	if options.Activates != nil {
		activationDateUTC = date.UnixTime(*options.Activates)
		secretAttributes.NotBefore = &activationDateUTC
	}

	if options.Expires != nil {
		expireDateUTC = date.UnixTime(*options.Expires)
		secretAttributes.Expires = &expireDateUTC
	}

	// Initialize secret parameters
	secretParams := keyvaults.SecretSetParameters{
		Value:            &stringSecret,
		SecretAttributes: &secretAttributes,
	}

	if _, err := k.KeyVaultClient.GetSecret(ctx, vaultBaseURL, secretName, secretVersion); err == nil {
		return fmt.Errorf("secret already exists %v", err)
	}

	_, err = k.KeyVaultClient.SetSecret(ctx, vaultBaseURL, secretName, secretParams)

	return err

}

// Upsert updates a key in KeyVault even if it exists already, creates if it doesn't exist
func (k *KeyvaultSecretClient) Upsert(ctx context.Context, key types.NamespacedName, data map[string][]byte, opts ...secrets.SecretOption) error {
	//return nil
	options := &secrets.Options{}
	for _, opt := range opts {
		opt(options)
	}

	vaultBaseURL := getVaultsURL(ctx, k.KeyVaultName)
	var secretName string
	if len(key.Namespace) != 0 {
		secretName = key.Namespace + "-" + key.Name
	} else {
		secretName = key.Name
	}
	//secretVersion := ""
	enabled := true

	var activationDateUTC date.UnixTime
	var expireDateUTC date.UnixTime

	// Convert the map into a string as that's what a KeyVault secret takes
	jsonData, err := json.Marshal(data)
	if err != nil {
		return err
	}
	stringSecret := string(jsonData)

	// Initialize secret attributes
	secretAttributes := keyvaults.SecretAttributes{
		Enabled: &enabled,
	}

	if options.Activates != nil {
		activationDateUTC = date.UnixTime(*options.Activates)
		secretAttributes.NotBefore = &activationDateUTC
	}

	if options.Expires != nil {
		expireDateUTC = date.UnixTime(*options.Expires)
		secretAttributes.Expires = &expireDateUTC
	}

	// Initialize secret parameters
	secretParams := keyvaults.SecretSetParameters{
		Value:            &stringSecret,
		SecretAttributes: &secretAttributes,
	}

	/*if _, err := k.KeyVaultClient.GetSecret(ctx, vaultBaseURL, secretName, secretVersion); err == nil {
		// If secret exists we delete it and recreate it again
		_, err = k.KeyVaultClient.DeleteSecret(ctx, vaultBaseURL, secretName)
		if err != nil {
			return fmt.Errorf("Upsert failed: Trying to delete existing secret failed with %v", err)
		}
	}*/

	_, err = k.KeyVaultClient.SetSecret(ctx, vaultBaseURL, secretName, secretParams)

	return err
}

// Delete deletes a key in KeyVault
func (k *KeyvaultSecretClient) Delete(ctx context.Context, key types.NamespacedName) error {
	vaultBaseURL := getVaultsURL(ctx, k.KeyVaultName)
	var secretName string
	if len(key.Namespace) != 0 {
		secretName = key.Namespace + "-" + key.Name
	} else {
		secretName = key.Name
	}
	_, err := k.KeyVaultClient.DeleteSecret(ctx, vaultBaseURL, secretName)
	return err
}

// Get gets a key from KeyVault
func (k *KeyvaultSecretClient) Get(ctx context.Context, key types.NamespacedName) (map[string][]byte, error) {
	vaultBaseURL := getVaultsURL(ctx, k.KeyVaultName)
	var secretName string
	if len(key.Namespace) != 0 {
		secretName = key.Namespace + "-" + key.Name
	} else {
		secretName = key.Name
	}

	secretVersion := ""
	data := map[string][]byte{}

	result, err := k.KeyVaultClient.GetSecret(ctx, vaultBaseURL, secretName, secretVersion)

	if err != nil {
		return data, fmt.Errorf("secret does not exist" + err.Error())
	}

	stringSecret := *result.Value

	// Convert the data from json string to map and return
	json.Unmarshal([]byte(stringSecret), &data)

	return data, err
}

// Create creates a key in KeyVault if it does not exist already
func (k *KeyvaultSecretClient) CreateEncryptionKey(ctx context.Context, name string) error {
	vaultBaseURL := getVaultsURL(ctx, k.KeyVaultName)
	var ksize int32 = 4096
	kops := keyvault.PossibleJSONWebKeyOperationValues()
	katts := keyvault.KeyAttributes{
		Enabled: to.BoolPtr(true),
	}
	params := keyvault.KeyCreateParameters{
		Kty:           keyvault.RSA,
		KeySize:       &ksize,
		KeyOps:        &kops,
		KeyAttributes: &katts,
	}
	k.KeyVaultClient.CreateKey(ctx, vaultBaseURL, name, params)
	return nil

}
