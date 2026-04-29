package deploy

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EnsureAppSecret creates or updates a K8s Opaque secret named "<name>-env"
// containing the given key/value pairs. This is how captain injects environment
// variables (from exposes + secrets) into app pods.
func EnsureAppSecret(ctx context.Context, name, namespace, kubeContext string, data map[string]string) error {
	secretName := name + "-env"

	if data == nil {
		data = make(map[string]string)
	}

	clientset, err := newKubeClient(kubeContext)
	if err != nil {
		return err
	}

	if err := ensureNamespace(ctx, clientset, namespace); err != nil {
		return err
	}

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: namespace,
			Labels:    map[string]string{"app.kubernetes.io/managed-by": "captain"},
		},
		Type:       corev1.SecretTypeOpaque,
		StringData: data,
	}

	secrets := clientset.CoreV1().Secrets(namespace)
	existing, err := secrets.Get(ctx, secretName, metav1.GetOptions{})
	if k8serrors.IsNotFound(err) {
		if _, err := secrets.Create(ctx, secret, metav1.CreateOptions{}); err != nil {
			return fmt.Errorf("creating secret %s: %w", secretName, err)
		}
		return nil
	} else if err != nil {
		return fmt.Errorf("checking secret %s: %w", secretName, err)
	}

	existing.StringData = data
	if _, err := secrets.Update(ctx, existing, metav1.UpdateOptions{}); err != nil {
		return fmt.Errorf("updating secret %s: %w", secretName, err)
	}
	return nil
}
