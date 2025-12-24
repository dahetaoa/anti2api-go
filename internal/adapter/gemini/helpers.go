package gemini

import (
	"anti2api-golang/internal/store"
	"anti2api-golang/internal/utils"
)

// getProjectID 获取项目ID
func getProjectID(account *store.Account) string {
	if account.ProjectID != "" {
		return account.ProjectID
	}
	return utils.GenerateProjectID()
}
