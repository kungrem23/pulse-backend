package moderation

type createReportRequest struct {
	ToUserID string `json:"to_user_id"`
	Reason   string `json:"reason"`
}

type adminMessageRequest struct {
	Message string `json:"message"`
}
