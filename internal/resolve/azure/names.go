package azure

import (
	"strings"

	"github.com/mooncitizen/togen/internal/resolve"
)

const storageAccountNameMax = 24

// Storage account names are 3 to 24 lowercase alphanumerics. When the three parts do not fit
// the project is cut first, since the node is what tells accounts in a project apart.
func storageAccountName(ctx *resolve.Context, node string) string {
	project := alphanumeric(ctx.Project.Name)
	env := alphanumeric(ctx.Project.Environment)
	node = alphanumeric(node)
	if over := len(project) + len(env) + len(node) - storageAccountNameMax; over > 0 {
		cut := min(over, len(project))
		project = project[:len(project)-cut]
		node = node[:len(node)-min(over-cut, len(node))]
	}
	return project + env + node
}

func alphanumeric(s string) string {
	return strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			return r
		}
		return -1
	}, strings.ToLower(s))
}
