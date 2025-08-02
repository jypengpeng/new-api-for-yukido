package common

import (
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"net/smtp"
	"slices"
	"strings"
	"time"
)

func generateMessageID() (string, error) {
	split := strings.Split(SMTPFrom, "@")
	if len(split) < 2 {
		return "", fmt.Errorf("invalid SMTP account")
	}
	domain := strings.Split(SMTPFrom, "@")[1]
	return fmt.Sprintf("<%d.%s@%s>", time.Now().UnixNano(), GetRandomString(12), domain), nil
}

func SendEmail(subject string, receiver string, content string) error {
	SysLog(fmt.Sprintf("开始发送邮件 - 收件人: %s, 主题: %s", receiver, subject))
	
	// 检查SMTP配置
	if SMTPFrom == "" { // for compatibility
		SMTPFrom = SMTPAccount
		SysLog(fmt.Sprintf("SMTPFrom为空，使用SMTPAccount作为发件人: %s", SMTPFrom))
	}
	
	// 生成Message-ID
	id, err2 := generateMessageID()
	if err2 != nil {
		SysError(fmt.Sprintf("生成Message-ID失败: %s", err2.Error()))
		return err2
	}
	SysLog(fmt.Sprintf("生成Message-ID成功: %s", id))
	
	// 检查SMTP服务器配置
	if SMTPServer == "" && SMTPAccount == "" {
		err := fmt.Errorf("SMTP 服务器未配置")
		SysError(fmt.Sprintf("SMTP配置检查失败: %s", err.Error()))
		return err
	}
	SysLog(fmt.Sprintf("SMTP配置检查通过 - 服务器: %s, 端口: %d, 账户: %s, SSL: %v", SMTPServer, SMTPPort, SMTPAccount, SMTPSSLEnabled))
	
	// 编码邮件主题
	encodedSubject := fmt.Sprintf("=?UTF-8?B?%s?=", base64.StdEncoding.EncodeToString([]byte(subject)))
	SysLog(fmt.Sprintf("邮件主题编码完成: %s", encodedSubject))
	
	// 构建邮件内容
	mail := []byte(fmt.Sprintf("To: %s\r\n"+
		"From: %s<%s>\r\n"+
		"Subject: %s\r\n"+
		"Date: %s\r\n"+
		"Message-ID: %s\r\n"+ // 添加 Message-ID 头
		"Content-Type: text/html; charset=UTF-8\r\n\r\n%s\r\n",
		receiver, SystemName, SMTPFrom, encodedSubject, time.Now().Format(time.RFC1123Z), id, content))
	SysLog(fmt.Sprintf("邮件内容构建完成，长度: %d 字节", len(mail)))
	
	// 创建认证
	auth := smtp.PlainAuth("", SMTPAccount, SMTPToken, SMTPServer)
	SysLog(fmt.Sprintf("创建SMTP认证 - 账户: %s, 服务器: %s", SMTPAccount, SMTPServer))
	
	addr := fmt.Sprintf("%s:%d", SMTPServer, SMTPPort)
	to := strings.Split(receiver, ";")
	SysLog(fmt.Sprintf("准备发送到地址: %s, 收件人列表: %v", addr, to))
	
	var err error
	
	// SSL/TLS连接处理
	if SMTPPort == 465 || SMTPSSLEnabled {
		SysLog(fmt.Sprintf("使用SSL/TLS连接 - 端口: %d, SSL启用: %v", SMTPPort, SMTPSSLEnabled))
		
		tlsConfig := &tls.Config{
			InsecureSkipVerify: true,
			ServerName:         SMTPServer,
		}
		SysLog(fmt.Sprintf("创建TLS配置 - 服务器名: %s, 跳过验证: %v", SMTPServer, tlsConfig.InsecureSkipVerify))
		
		// 建立TLS连接
		SysLog(fmt.Sprintf("尝试建立TLS连接到: %s:%d", SMTPServer, SMTPPort))
		conn, err := tls.Dial("tcp", fmt.Sprintf("%s:%d", SMTPServer, SMTPPort), tlsConfig)
		if err != nil {
			SysError(fmt.Sprintf("TLS连接失败: %s", err.Error()))
			return err
		}
		SysLog("TLS连接建立成功")
		
		// 创建SMTP客户端
		SysLog("创建SMTP客户端")
		client, err := smtp.NewClient(conn, SMTPServer)
		if err != nil {
			SysError(fmt.Sprintf("创建SMTP客户端失败: %s", err.Error()))
			return err
		}
		SysLog("SMTP客户端创建成功")
		defer func() {
			client.Close()
			SysLog("SMTP客户端连接已关闭")
		}()
		
		// 进行认证
		SysLog("开始SMTP认证")
		if err = client.Auth(auth); err != nil {
			SysError(fmt.Sprintf("SMTP认证失败: %s", err.Error()))
			return err
		}
		SysLog("SMTP认证成功")
		
		// 设置发件人
		SysLog(fmt.Sprintf("设置发件人: %s", SMTPFrom))
		if err = client.Mail(SMTPFrom); err != nil {
			SysError(fmt.Sprintf("设置发件人失败: %s", err.Error()))
			return err
		}
		SysLog("发件人设置成功")
		
		// 设置收件人
		receiverEmails := strings.Split(receiver, ";")
		for i, receiver := range receiverEmails {
			SysLog(fmt.Sprintf("设置收件人 %d: %s", i+1, receiver))
			if err = client.Rcpt(receiver); err != nil {
				SysError(fmt.Sprintf("设置收件人 %s 失败: %s", receiver, err.Error()))
				return err
			}
			SysLog(fmt.Sprintf("收件人 %s 设置成功", receiver))
		}
		
		// 发送邮件数据
		SysLog("开始发送邮件数据")
		w, err := client.Data()
		if err != nil {
			SysError(fmt.Sprintf("获取数据写入器失败: %s", err.Error()))
			return err
		}
		SysLog("数据写入器获取成功")
		
		_, err = w.Write(mail)
		if err != nil {
			SysError(fmt.Sprintf("写入邮件数据失败: %s", err.Error()))
			return err
		}
		SysLog(fmt.Sprintf("邮件数据写入成功，%d 字节", len(mail)))
		
		err = w.Close()
		if err != nil {
			SysError(fmt.Sprintf("关闭数据写入器失败: %s", err.Error()))
			return err
		}
		SysLog("数据写入器关闭成功")
		
		SysLog("SSL/TLS邮件发送完成")
		
	} else if isOutlookServer(SMTPAccount) || slices.Contains(EmailLoginAuthServerList, SMTPServer) {
		// 特殊认证方式处理
		SysLog(fmt.Sprintf("使用特殊认证方式 - Outlook服务器: %v, 登录认证服务器列表: %v", isOutlookServer(SMTPAccount), slices.Contains(EmailLoginAuthServerList, SMTPServer)))
		auth = LoginAuth(SMTPAccount, SMTPToken)
		SysLog("创建登录认证成功")
		
		SysLog(fmt.Sprintf("使用smtp.SendMail发送邮件到: %s", addr))
		err = smtp.SendMail(addr, auth, SMTPFrom, to, mail)
		if err != nil {
			SysError(fmt.Sprintf("smtp.SendMail发送失败: %s", err.Error()))
			return err
		}
		SysLog("特殊认证方式邮件发送完成")
		
	} else {
		// 标准SMTP发送
		SysLog("使用标准SMTP发送方式")
		SysLog(fmt.Sprintf("使用smtp.SendMail发送邮件到: %s", addr))
		err = smtp.SendMail(addr, auth, SMTPFrom, to, mail)
		if err != nil {
			SysError(fmt.Sprintf("smtp.SendMail发送失败: %s", err.Error()))
			return err
		}
		SysLog("标准SMTP邮件发送完成")
	}
	
	SysLog(fmt.Sprintf("邮件发送成功 - 收件人: %s, 主题: %s", receiver, subject))
	return err
}
