package deploy

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/crherman7/captain/internal/config"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EnsureRegistrySecret creates or updates a docker-registry secret in the given namespace.
// Returns the secret name if created, or empty string if no secret is needed.
func EnsureRegistrySecret(ctx context.Context, reg *config.RegistryConfig, namespace, kubeContext string) (string, error) {
	if reg == nil || !reg.NeedsSecret() {
		return "", nil
	}

	secretName := reg.SecretName()

	clientset, err := newKubeClient(kubeContext)
	if err != nil {
		return "", err
	}

	if err := ensureNamespace(ctx, clientset, namespace); err != nil {
		return "", err
	}

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
			Labels:    map[string]string{"app.kubernetes.io/managed-by": "captain"},
		},
		Type: corev1.SecretTypeDockerConfigJson,
		Data: map[string][]byte{corev1.DockerConfigJsonKey: dockerConfigJSON},
	}

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

	existing.Data = secret.Data
	if _, err := secrets.Update(ctx, existing, metav1.UpdateOptions{}); err != nil {
		return "", fmt.Errorf("updating registry secret: %w", err)
	}
	return secretName, nil
}
