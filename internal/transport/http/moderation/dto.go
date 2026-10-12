package moderation

type createReportRequest struct {
	ToUserID string `json:"to_user_id"`
	Reason   string `json:"reason"`
	Comment  string `json:"comment,omitempty"`
}

type processReportRequest struct {
	Status     string `json:"status"`
	Resolution string `json:"resolution"`
	Comment    string `json:"comment,omitempty"`
}

type adminMessageRequest struct {
	Message string `json:"message"`
}

type notificationRequest struct {
	Title   string `json:"title"`
	Message string `json:"message"`
}
