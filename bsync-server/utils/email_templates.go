package utils

import (
	"fmt"
)

// GetNewUserEmailTemplate returns HTML template for new user credentials
func GetNewUserEmailTemplate(fullname, username, password, loginURL string) string {
	return fmt.Sprintf(`
<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Welcome to BSync</title>
    <style>
        body {
            margin: 0;
            padding: 0;
            font-family: 'Segoe UI', Tahoma, Geneva, Verdana, sans-serif;
            background-color: #f4f7fa;
        }
        .email-container {
            max-width: 600px;
            margin: 40px auto;
            background-color: #ffffff;
            border-radius: 12px;
            overflow: hidden;
            box-shadow: 0 4px 12px rgba(0, 0, 0, 0.08);
        }
        .email-header {
            background: linear-gradient(135deg, #667eea 0%%, #764ba2 100%%);
            padding: 40px 30px;
            text-align: center;
        }
        .email-header h1 {
            color: #ffffff;
            margin: 0;
            font-size: 28px;
            font-weight: 600;
        }
        .email-header p {
            color: #e0e7ff;
            margin: 10px 0 0 0;
            font-size: 16px;
        }
        .email-body {
            padding: 40px 30px;
        }
        .greeting {
            font-size: 18px;
            color: #333333;
            margin-bottom: 20px;
        }
        .message {
            font-size: 15px;
            color: #666666;
            line-height: 1.6;
            margin-bottom: 30px;
        }
        .credentials-box {
            background-color: #f8f9fc;
            border-left: 4px solid #667eea;
            border-radius: 8px;
            padding: 25px;
            margin: 30px 0;
        }
        .credentials-box h3 {
            margin: 0 0 15px 0;
            color: #333333;
            font-size: 16px;
            font-weight: 600;
        }
        .credential-item {
            display: flex;
            justify-content: space-between;
            align-items: center;
            padding: 12px 0;
            border-bottom: 1px solid #e2e8f0;
        }
        .credential-item:last-child {
            border-bottom: none;
        }
        .credential-label {
            font-weight: 500;
            color: #555555;
            font-size: 14px;
        }
        .credential-value {
            font-family: 'Courier New', monospace;
            font-size: 15px;
            color: #333333;
            background-color: #ffffff;
            padding: 8px 12px;
            border-radius: 6px;
            border: 1px solid #e2e8f0;
            font-weight: 600;
        }
        .password-value {
            color: #dc2626;
        }
        .security-notice {
            background-color: #fef3c7;
            border-left: 4px solid #f59e0b;
            border-radius: 8px;
            padding: 15px 20px;
            margin: 30px 0;
        }
        .security-notice p {
            margin: 0;
            font-size: 14px;
            color: #92400e;
            line-height: 1.5;
        }
        .security-notice strong {
            color: #78350f;
        }
        .cta-button {
            display: inline-block;
            background: linear-gradient(135deg, #667eea 0%%, #764ba2 100%%);
            color: #ffffff;
            text-decoration: none;
            padding: 14px 32px;
            border-radius: 8px;
            font-weight: 600;
            font-size: 16px;
            margin: 20px 0;
            transition: transform 0.2s;
        }
        .cta-button:hover {
            transform: translateY(-2px);
        }
        .login-url {
            font-size: 14px;
            color: #666666;
            margin-top: 15px;
            word-break: break-all;
        }
        .login-url a {
            color: #667eea;
            text-decoration: none;
        }
        .email-footer {
            background-color: #f8f9fc;
            padding: 30px;
            text-align: center;
            border-top: 1px solid #e2e8f0;
        }
        .email-footer p {
            margin: 5px 0;
            font-size: 13px;
            color: #888888;
        }
        .divider {
            height: 1px;
            background-color: #e2e8f0;
            margin: 25px 0;
        }
        .icon {
            display: inline-block;
            width: 20px;
            height: 20px;
            vertical-align: middle;
            margin-right: 8px;
        }
    </style>
</head>
<body>
    <div class="email-container">
        <div class="email-header">
            <h1>🎉 Welcome to BSync</h1>
            <p>Your account has been successfully created</p>
        </div>

        <div class="email-body">
            <p class="greeting">Hello <strong>%s</strong>,</p>

            <p class="message">
                Your account has been created successfully. Below are your login credentials to access the BSync platform.
                Please keep this information secure and do not share it with anyone.
            </p>

            <div class="credentials-box">
                <h3>🔐 Your Login Credentials</h3>
                <div class="credential-item">
                    <span class="credential-label">Username:</span>
                    <span class="credential-value">%s</span>
                </div>
                <div class="credential-item">
                    <span class="credential-label">Temporary Password:</span>
                    <span class="credential-value password-value">%s</span>
                </div>
            </div>

            <div class="security-notice">
                <p>
                    <strong>⚠️ Important Security Notice:</strong><br>
                    For security reasons, we strongly recommend that you change your password immediately after your first login.
                    This temporary password should only be used for initial access.
                </p>
            </div>

            <div class="divider"></div>

            <div style="text-align: center;">
                <a href="%s" class="cta-button">Login to BSync</a>
                <p class="login-url">
                    Or copy and paste this URL into your browser:<br>
                    <a href="%s">%s</a>
                </p>
            </div>

            <div class="divider"></div>

            <p class="message">
                If you have any questions or need assistance, please don't hesitate to contact our support team.
            </p>
        </div>

        <div class="email-footer">
            <p><strong>BSync - Business Synchronization Platform</strong></p>
            <p>This is an automated message, please do not reply to this email.</p>
            <p style="margin-top: 15px; color: #aaaaaa;">© 2025 BSync. All rights reserved.</p>
        </div>
    </div>
</body>
</html>
`, fullname, username, password, loginURL, loginURL, loginURL)
}

// GetPasswordResetEmailTemplate returns HTML template for password reset
func GetPasswordResetEmailTemplate(fullname, username, newPassword string) string {
	return fmt.Sprintf(`
<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Password Reset - BSync</title>
    <style>
        body {
            margin: 0;
            padding: 0;
            font-family: 'Segoe UI', Tahoma, Geneva, Verdana, sans-serif;
            background-color: #f4f7fa;
        }
        .email-container {
            max-width: 600px;
            margin: 40px auto;
            background-color: #ffffff;
            border-radius: 12px;
            overflow: hidden;
            box-shadow: 0 4px 12px rgba(0, 0, 0, 0.08);
        }
        .email-header {
            background: linear-gradient(135deg, #ef4444 0%%, #dc2626 100%%);
            padding: 40px 30px;
            text-align: center;
        }
        .email-header h1 {
            color: #ffffff;
            margin: 0;
            font-size: 28px;
            font-weight: 600;
        }
        .email-header p {
            color: #fee2e2;
            margin: 10px 0 0 0;
            font-size: 16px;
        }
        .email-body {
            padding: 40px 30px;
        }
        .greeting {
            font-size: 18px;
            color: #333333;
            margin-bottom: 20px;
        }
        .message {
            font-size: 15px;
            color: #666666;
            line-height: 1.6;
            margin-bottom: 30px;
        }
        .credentials-box {
            background-color: #fef2f2;
            border-left: 4px solid #ef4444;
            border-radius: 8px;
            padding: 25px;
            margin: 30px 0;
        }
        .credentials-box h3 {
            margin: 0 0 15px 0;
            color: #333333;
            font-size: 16px;
            font-weight: 600;
        }
        .credential-item {
            display: flex;
            justify-content: space-between;
            align-items: center;
            padding: 12px 0;
            border-bottom: 1px solid #fecaca;
        }
        .credential-item:last-child {
            border-bottom: none;
        }
        .credential-label {
            font-weight: 500;
            color: #555555;
            font-size: 14px;
        }
        .credential-value {
            font-family: 'Courier New', monospace;
            font-size: 15px;
            color: #dc2626;
            background-color: #ffffff;
            padding: 8px 12px;
            border-radius: 6px;
            border: 1px solid #fecaca;
            font-weight: 600;
        }
        .security-notice {
            background-color: #fef3c7;
            border-left: 4px solid #f59e0b;
            border-radius: 8px;
            padding: 15px 20px;
            margin: 30px 0;
        }
        .security-notice p {
            margin: 0;
            font-size: 14px;
            color: #92400e;
            line-height: 1.5;
        }
        .email-footer {
            background-color: #f8f9fc;
            padding: 30px;
            text-align: center;
            border-top: 1px solid #e2e8f0;
        }
        .email-footer p {
            margin: 5px 0;
            font-size: 13px;
            color: #888888;
        }
    </style>
</head>
<body>
    <div class="email-container">
        <div class="email-header">
            <h1>🔒 Password Reset</h1>
            <p>Your password has been reset</p>
        </div>

        <div class="email-body">
            <p class="greeting">Hello <strong>%s</strong>,</p>

            <p class="message">
                Your password has been reset by an administrator. Below are your new login credentials.
            </p>

            <div class="credentials-box">
                <h3>🔐 Your New Credentials</h3>
                <div class="credential-item">
                    <span class="credential-label">Username:</span>
                    <span class="credential-value">%s</span>
                </div>
                <div class="credential-item">
                    <span class="credential-label">New Password:</span>
                    <span class="credential-value">%s</span>
                </div>
            </div>

            <div class="security-notice">
                <p>
                    <strong>⚠️ Security Recommendation:</strong><br>
                    Please change this password immediately after logging in to ensure your account security.
                </p>
            </div>
        </div>

        <div class="email-footer">
            <p><strong>BSync - Business Synchronization Platform</strong></p>
            <p>If you did not request this password reset, please contact support immediately.</p>
            <p style="margin-top: 15px; color: #aaaaaa;">© 2025 BSync. All rights reserved.</p>
        </div>
    </div>
</body>
</html>
`, fullname, username, newPassword)
}

// SendNewUserEmail sends welcome email with credentials to new user
func SendNewUserEmail(to, fullname, username, password, loginURL string) error {
	subject := "Welcome to BSync - Your Account Credentials"
	htmlBody := GetNewUserEmailTemplate(fullname, username, password, loginURL)

	return SendHTMLEmail([]string{to}, subject, htmlBody)
}

// SendPasswordResetEmail sends password reset email
func SendPasswordResetEmail(to, fullname, username, newPassword string) error {
	subject := "BSync - Password Reset Notification"
	htmlBody := GetPasswordResetEmailTemplate(fullname, username, newPassword)

	return SendHTMLEmail([]string{to}, subject, htmlBody)
}
