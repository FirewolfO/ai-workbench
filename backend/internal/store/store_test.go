package store

import (
	"path/filepath"
	"testing"

	"ai-workbench/internal/model"
)

func TestBuiltinOpenAIUsesSolModel(t *testing.T) {
	for _, provider := range builtinProviders {
		if provider.ID == "prv_builtin_openai" {
			if provider.DefaultModel != "gpt-5.6-sol" {
				t.Fatalf("OpenAI default model = %q", provider.DefaultModel)
			}
			return
		}
	}
	t.Fatal("builtin OpenAI provider not found")
}

func TestSeedBuiltinProvidersOnceWithoutOverwritingExistingData(t *testing.T) {
	database, err := Open(filepath.Join(t.TempDir(), "store.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })

	existing := model.Provider{
		ID: "prv_existing", OwnerID: "admin", Name: "公司 OpenAI",
		BaseURL: "https://api.openai.com/v1", DefaultModel: "company-model",
		APIKeyCiphertext: "encrypted", Enabled: true,
	}
	if err := database.DB.Create(&existing).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.SeedBuiltinProviders(); err != nil {
		t.Fatal(err)
	}

	var providers []model.Provider
	if err := database.DB.Order("id ASC").Find(&providers).Error; err != nil {
		t.Fatal(err)
	}
	if len(providers) != len(builtinProviders) {
		t.Fatalf("provider count = %d, want %d", len(providers), len(builtinProviders))
	}
	var openAI model.Provider
	if err := database.DB.First(&openAI, "id = ?", existing.ID).Error; err != nil {
		t.Fatal(err)
	}
	if openAI.Name != existing.Name || openAI.DefaultModel != existing.DefaultModel || openAI.APIKeyCiphertext != existing.APIKeyCiphertext || !openAI.Enabled {
		t.Fatalf("existing provider was changed: %#v", openAI)
	}
	for _, provider := range providers {
		if provider.ID == existing.ID {
			continue
		}
		if provider.Enabled || provider.APIKeyCiphertext != "" {
			t.Fatalf("builtin provider should be disabled without a key: %#v", provider)
		}
	}

	deletedID := "prv_builtin_gemini"
	if err := database.DB.Delete(&model.Provider{}, "id = ?", deletedID).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.DB.Model(&model.Provider{}).Where("id = ?", "prv_builtin_deepseek").Update("default_model", "custom-model").Error; err != nil {
		t.Fatal(err)
	}
	if err := database.SeedBuiltinProviders(); err != nil {
		t.Fatal(err)
	}
	var count int64
	if err := database.DB.Model(&model.Provider{}).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != int64(len(builtinProviders)-1) {
		t.Fatalf("provider count after repeat seed = %d, want %d", count, len(builtinProviders)-1)
	}
	var deepSeek model.Provider
	if err := database.DB.First(&deepSeek, "id = ?", "prv_builtin_deepseek").Error; err != nil {
		t.Fatal(err)
	}
	if deepSeek.DefaultModel != "custom-model" {
		t.Fatalf("repeat seed overwrote provider: %#v", deepSeek)
	}
}
