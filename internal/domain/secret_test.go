package domain

import (
	"testing"
	"time"
)

func TestSecretToSummary(t *testing.T) {
	now := time.Now()
	secret := &Secret{
		ID:          "test-secret",
		Name:        "Test Secret",
		Type:        SecretTypeSSHKey,
		Source:      SecretSourceOperator,
		Description: "Test description",
		Data: map[string]string{
			"private_key": "-----BEGIN RSA PRIVATE KEY-----",
			"username":    "admin",
			"passphrase":  "secret123",
		},
		Metadata: map[string]string{
			"purpose": "testing",
		},
		Immutable:     false,
		CreatedAt:     now,
		UpdatedAt:     now,
		UsageCount:    5,
		Status:        SecretStatusValid,
		StatusMessage: "Working correctly",
	}

	t.Run("creates summary without sensitive data", func(t *testing.T) {
		summary := secret.ToSummary()

		if summary.ID != secret.ID {
			t.Errorf("expected ID %s, got %s", secret.ID, summary.ID)
		}
		if summary.Name != secret.Name {
			t.Errorf("expected Name %s, got %s", secret.Name, summary.Name)
		}
		if summary.Type != secret.Type {
			t.Errorf("expected Type %s, got %s", secret.Type, summary.Type)
		}
		if summary.Source != secret.Source {
			t.Errorf("expected Source %s, got %s", secret.Source, summary.Source)
		}
		if summary.UsageCount != secret.UsageCount {
			t.Errorf("expected UsageCount %d, got %d", secret.UsageCount, summary.UsageCount)
		}
	})

	t.Run("includes data keys but not values", func(t *testing.T) {
		summary := secret.ToSummary()

		if len(summary.DataKeys) != 3 {
			t.Errorf("expected 3 data keys, got %d", len(summary.DataKeys))
		}

		// Verify keys are present
		keyMap := make(map[string]bool)
		for _, key := range summary.DataKeys {
			keyMap[key] = true
		}

		if !keyMap["private_key"] {
			t.Error("expected 'private_key' in data keys")
		}
		if !keyMap["username"] {
			t.Error("expected 'username' in data keys")
		}
		if !keyMap["passphrase"] {
			t.Error("expected 'passphrase' in data keys")
		}
	})

	t.Run("handles empty data map", func(t *testing.T) {
		secret := &Secret{
			ID:   "empty",
			Name: "Empty",
			Type: SecretTypeGeneric,
			Data: map[string]string{},
		}

		summary := secret.ToSummary()
		if len(summary.DataKeys) != 0 {
			t.Errorf("expected 0 data keys, got %d", len(summary.DataKeys))
		}
	})

	t.Run("handles nil data map", func(t *testing.T) {
		secret := &Secret{
			ID:   "nil",
			Name: "Nil",
			Type: SecretTypeGeneric,
			Data: nil,
		}

		summary := secret.ToSummary()
		if summary.DataKeys == nil {
			t.Error("expected DataKeys to be initialized")
		}
		if len(summary.DataKeys) != 0 {
			t.Errorf("expected 0 data keys, got %d", len(summary.DataKeys))
		}
	})
}

func TestSecretTypes(t *testing.T) {
	t.Run("all secret types are defined", func(t *testing.T) {
		types := []SecretType{
			SecretTypeSSHKey,
			SecretTypeSSHPassword,
			SecretTypeSNMPCommunity,
			SecretTypeSNMPv3,
			SecretTypeAPIToken,
			SecretTypeDNS,
			SecretTypeGeneric,
		}

		for _, secretType := range types {
			if secretType == "" {
				t.Error("expected secret type to have non-empty value")
			}
		}
	})
}

func TestSecretSources(t *testing.T) {
	t.Run("secret sources are defined", func(t *testing.T) {
		sources := []SecretSource{
			SecretSourceMounted,
			SecretSourceOperator,
		}

		for _, source := range sources {
			if source == "" {
				t.Error("expected secret source to have non-empty value")
			}
		}
	})
}

func TestSecretStatus(t *testing.T) {
	t.Run("secret statuses are defined", func(t *testing.T) {
		statuses := []SecretStatus{
			SecretStatusUnknown,
			SecretStatusValid,
			SecretStatusInvalid,
			SecretStatusExpired,
		}

		for _, status := range statuses {
			if status == "" {
				t.Error("expected secret status to have non-empty value")
			}
		}
	})
}

func TestGetSecretTypeInfos(t *testing.T) {
	infos := GetSecretTypeInfos()

	t.Run("returns info for all secret types", func(t *testing.T) {
		if len(infos) != 7 {
			t.Errorf("expected 7 secret type infos, got %d", len(infos))
		}
	})

	t.Run("SSH key info is complete", func(t *testing.T) {
		var sshKeyInfo *SecretTypeInfo
		for i := range infos {
			if infos[i].Type == SecretTypeSSHKey {
				sshKeyInfo = &infos[i]
				break
			}
		}

		if sshKeyInfo == nil {
			t.Fatal("expected to find SSH key info")
		}

		if sshKeyInfo.Name == "" {
			t.Error("expected Name to be set")
		}
		if sshKeyInfo.Description == "" {
			t.Error("expected Description to be set")
		}
		if len(sshKeyInfo.Fields) != 3 {
			t.Errorf("expected 3 fields, got %d", len(sshKeyInfo.Fields))
		}

		// Check for private_key field
		var privateKeyField *SecretFieldInfo
		for i := range sshKeyInfo.Fields {
			if sshKeyInfo.Fields[i].Key == "private_key" {
				privateKeyField = &sshKeyInfo.Fields[i]
				break
			}
		}

		if privateKeyField == nil {
			t.Fatal("expected to find private_key field")
		}
		if !privateKeyField.Required {
			t.Error("expected private_key to be required")
		}
		if !privateKeyField.Sensitive {
			t.Error("expected private_key to be sensitive")
		}
		if privateKeyField.Type != "textarea" {
			t.Errorf("expected type 'textarea', got %s", privateKeyField.Type)
		}
	})

	t.Run("SNMP community info has default value", func(t *testing.T) {
		var snmpInfo *SecretTypeInfo
		for i := range infos {
			if infos[i].Type == SecretTypeSNMPCommunity {
				snmpInfo = &infos[i]
				break
			}
		}

		if snmpInfo == nil {
			t.Fatal("expected to find SNMP community info")
		}

		if len(snmpInfo.Fields) == 0 {
			t.Fatal("expected at least one field")
		}

		communityField := snmpInfo.Fields[0]
		if communityField.Default != "public" {
			t.Errorf("expected default 'public', got %s", communityField.Default)
		}
	})

	t.Run("SNMPv3 info has select fields with options", func(t *testing.T) {
		var snmpv3Info *SecretTypeInfo
		for i := range infos {
			if infos[i].Type == SecretTypeSNMPv3 {
				snmpv3Info = &infos[i]
				break
			}
		}

		if snmpv3Info == nil {
			t.Fatal("expected to find SNMPv3 info")
		}

		var securityLevelField *SecretFieldInfo
		for i := range snmpv3Info.Fields {
			if snmpv3Info.Fields[i].Key == "security_level" {
				securityLevelField = &snmpv3Info.Fields[i]
				break
			}
		}

		if securityLevelField == nil {
			t.Fatal("expected to find security_level field")
		}

		if securityLevelField.Type != "select" {
			t.Errorf("expected type 'select', got %s", securityLevelField.Type)
		}

		if len(securityLevelField.Options) != 3 {
			t.Errorf("expected 3 options, got %d", len(securityLevelField.Options))
		}
	})

	t.Run("all fields have required metadata", func(t *testing.T) {
		for _, info := range infos {
			if info.Type == "" {
				t.Errorf("secret type info has empty Type")
			}
			if info.Name == "" {
				t.Errorf("secret type %s has empty Name", info.Type)
			}

			for _, field := range info.Fields {
				if field.Key == "" {
					t.Errorf("field in %s has empty Key", info.Type)
				}
				if field.Name == "" {
					t.Errorf("field %s in %s has empty Name", field.Key, info.Type)
				}
				if field.Type == "" {
					t.Errorf("field %s in %s has empty Type", field.Key, info.Type)
				}

				// If type is select, should have options
				if field.Type == "select" && len(field.Options) == 0 {
					t.Errorf("select field %s in %s has no options", field.Key, info.Type)
				}
			}
		}
	})
}

func TestSecretRef(t *testing.T) {
	t.Run("secret ref with ID", func(t *testing.T) {
		ref := SecretRef{
			ID: "my-secret",
		}

		if ref.ID != "my-secret" {
			t.Errorf("expected ID 'my-secret', got %s", ref.ID)
		}
		if ref.Key != "" {
			t.Errorf("expected empty Key, got %s", ref.Key)
		}
	})

	t.Run("secret ref with ID and key", func(t *testing.T) {
		ref := SecretRef{
			ID:  "my-secret",
			Key: "username",
		}

		if ref.ID != "my-secret" {
			t.Errorf("expected ID 'my-secret', got %s", ref.ID)
		}
		if ref.Key != "username" {
			t.Errorf("expected Key 'username', got %s", ref.Key)
		}
	})
}
