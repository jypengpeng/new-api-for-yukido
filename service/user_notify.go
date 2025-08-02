package service

import (
	"fmt"
	"one-api/common"
	"one-api/dto"
	"one-api/model"
	"strings"
)

func NotifyRootUser(t string, subject string, content string) {
	user := model.GetRootUser().ToBaseUser()
	err := NotifyUser(user.Id, user.Email, user.GetSetting(), dto.NewNotify(t, subject, content, nil))
	if err != nil {
		common.SysError(fmt.Sprintf("failed to notify root user: %s", err.Error()))
	}
}

func NotifyUser(userId int, userEmail string, userSetting dto.UserSetting, data dto.Notify) error {
	notifyType := userSetting.NotifyType
	if notifyType == "" {
		notifyType = dto.NotifyTypeEmail
	}

	// Check notification limit
	canSend, err := CheckNotificationLimit(userId, data.Type)
	if err != nil {
		common.SysError(fmt.Sprintf("failed to check notification limit: %s", err.Error()))
		return err
	}
	if !canSend {
		return fmt.Errorf("notification limit exceeded for user %d with type %s", userId, notifyType)
	}

	switch notifyType {
	case dto.NotifyTypeEmail:
		// check setting email
		userEmail = userSetting.NotificationEmail
		if userEmail == "" {
			common.SysLog(fmt.Sprintf("user %d has no email, skip sending email", userId))
			return nil
		}
		return sendEmailNotify(userEmail, data)
	case dto.NotifyTypeWebhook:
		webhookURLStr := userSetting.WebhookUrl
		if webhookURLStr == "" {
			common.SysError(fmt.Sprintf("user %d has no webhook url, skip sending webhook", userId))
			return nil
		}

		// 获取 webhook secret
		webhookSecret := userSetting.WebhookSecret
		return SendWebhookNotify(webhookURLStr, webhookSecret, data)
	}
	return nil
}

func sendEmailNotify(userEmail string, data dto.Notify) error {
	common.SysLog(fmt.Sprintf("开始发送用户通知邮件 - 收件人: %s, 标题: %s", userEmail, data.Title))

	// make email content
	content := data.Content
	common.SysLog(fmt.Sprintf("原始邮件内容: %s", content))

	// 处理占位符
	if len(data.Values) > 0 {
		common.SysLog(fmt.Sprintf("开始处理占位符，占位符数量: %d", len(data.Values)))
		for i, value := range data.Values {
			common.SysLog(fmt.Sprintf("处理占位符 %d: %s -> %v", i+1, dto.ContentValueParam, value))
			content = strings.Replace(content, dto.ContentValueParam, fmt.Sprintf("%v", value), 1)
		}
		common.SysLog(fmt.Sprintf("占位符处理完成，最终内容: %s", content))
	} else {
		common.SysLog("没有占位符需要处理")
	}

	common.SysLog(fmt.Sprintf("准备调用SendEmail - 标题: %s, 收件人: %s", data.Title, userEmail))
	err := common.SendEmail(data.Title, userEmail, content)
	if err != nil {
		common.SysError(fmt.Sprintf("用户通知邮件发送失败: %s", err.Error()))
		return err
	}

	common.SysLog(fmt.Sprintf("用户通知邮件发送成功: %s", userEmail))
	return nil
}
