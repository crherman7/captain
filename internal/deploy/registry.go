package deploy

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/christopherherman/captain/internal/config"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

// EnsureRegistrySecret creates or updates a docker-registry secret in the given namespace.
// Returns the secret name if created, or empty string if no secret is needed.
func EnsureRegistrySecret(ctx context.Context, reg *config.RegistryConfig, namespace, kubeContext string) (string, error) {
	if reg == nil || !reg.NeedsSecret() {
		return "", nil
	}

	secretName := reg.SecretName()

	// Build kubeconfig
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	configOverrides := &clientcmd.ConfigOverrides{}
	if kubeContext != "" {
		configOverrides.CurrentContext = kubeContext
	}
	kubeConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, configOverrides)

	restConfig, err := kubeConfig.ClientConfig()
	if err != nil {
		return "", fmt.Errorf("building kubeconfig: %w", err)
	}

	clientset, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		return "", fmt.Errorf("creating kubernetes client: %w", err)
	}

	// Ensure namespace exists
	_, err = clientset.CoreV1().Namespaces().Get(ctx, namespace, metav1.GetOptions{})
	if k8serrors.IsNotFound(err) {
		ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}}
		if _, err := clientset.CoreV1().Namespaces().Create(ctx, ns, metav1.CreateOptions{}); err != nil && !k8serrors.IsAlreadyExists(err) {
			return "", fmt.Errorf("creating namespace %s: %w", namespace, err)
		}
	} else if err != nil {
		return "", fmt.Errorf("checking namespace %s: %w", namespace, err)
	}

	// Build docker config JSON
	dockerConfig := map[string]interface{}{
		"auths": map[string]interface{}{
			reg.Server: map[string]string{
				"username": reg.Username,
				"password": reg.Password,
			},
		},
	}
	dockerConfigJSON, err := json.Marshal(dockerConfig)
	if err != nil {
		return "", fmt.Errorf("marshaling docker config: %w", err)
	}

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: namespace,
			Labels: map[string]string{
				"app.kubernetes.io/managed-by": "captain",
			},
		},
		Type: corev1.SecretTypeDockerConfigJson,
		Data: map[string][]byte{
			corev1.DockerConfigJsonKey: dockerConfigJSON,
		},
	}

	// Create or update
	secrets := clientset.CoreV1().Secrets(namespace)
	existing, err := secrets.Get(ctx, secretName, metav1.GetOptions{})
	if k8serrors.IsNotFound(err) {
		if _, err := secrets.Create(ctx, secret, metav1.CreateOptions{}); err != nil {
			return "", fmt.Errorf("creating registry secret: %w", err)
		}
		return secretName, nil
	} else if err != nil {
		return "", fmt.Errorf("checking registry secret: %w", err)
	}

	// Update existing secret
	existing.Data = secret.Data
	if _, err := secrets.Update(ctx, existing, metav1.UpdateOptions{}); err != nil {
		return "", fmt.Errorf("updating registry secret: %w", err)
	}
	return secretName, nil
}
