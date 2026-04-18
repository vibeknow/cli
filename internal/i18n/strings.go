package i18n

func init() {
	Register("en", map[string]string{
		"err.profile.not_found":    "profile %q not found",
		"err.profile.duplicate":    "profile %q already exists",
		"err.config.invalid":       "config invalid: %s",
		"msg.profile.switched":     "active profile is now %q",
		"msg.profile.added":        "added profile %q",
		"msg.profile.removed":      "removed profile %q",
		"doctor.header":            "vibeknow doctor — environment and endpoint diagnostics",
		"doctor.ok":                "[ok] %s",
		"doctor.fail":              "[fail] %s: %s",
		"doctor.warn":              "[warn] %s: %s",
	})
	Register("zh", map[string]string{
		"err.profile.not_found":    "profile %q 不存在",
		"err.profile.duplicate":    "profile %q 已存在",
		"err.config.invalid":       "配置无效：%s",
		"msg.profile.switched":     "当前 profile 已切换为 %q",
		"msg.profile.added":        "已添加 profile %q",
		"msg.profile.removed":      "已删除 profile %q",
		"doctor.header":            "vibeknow doctor — 环境与 endpoint 诊断",
		"doctor.ok":                "[通过] %s",
		"doctor.fail":              "[失败] %s: %s",
		"doctor.warn":              "[警告] %s: %s",
	})
}
