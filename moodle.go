package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type Moodle struct {
	Base   string
	Token  string
	client *http.Client
}

var httpClient = &http.Client{Timeout: 60 * time.Second}

func normalizeBaseURL(u string) string {
	u = strings.TrimSpace(u)
	if u == "" {
		return u
	}
	if !strings.HasPrefix(u, "http://") && !strings.HasPrefix(u, "https://") {
		u = "https://" + u
	}
	// people often paste a page URL: keep everything before common Moodle paths
	for _, cut := range []string{"/login/", "/my/", "/course/", "/mod/", "/user/", "/index.php", "/?"} {
		if i := strings.Index(u, cut); i > 8 {
			u = u[:i]
		}
	}
	return strings.TrimRight(u, "/")
}

type MoodleError struct {
	Code    string
	Message string
}

func (e *MoodleError) Error() string { return e.Message + " (" + e.Code + ")" }

// GetToken logs in with username/password through the mobile app service.
func GetToken(base, username, password string) (string, error) {
	form := url.Values{"username": {username}, "password": {password}, "service": {"moodle_mobile_app"}}
	resp, err := httpClient.PostForm(base+"/login/token.php", form)
	if err != nil {
		return "", fmt.Errorf("تعذر الوصول لموقع المودل: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	var r struct {
		Token     string `json:"token"`
		Error     string `json:"error"`
		ErrorCode string `json:"errorcode"`
	}
	if err := json.Unmarshal(body, &r); err != nil {
		return "", errors.New("الرابط لا يبدو موقع مودل صحيح، تأكد من الرابط")
	}
	if r.Token == "" {
		switch r.ErrorCode {
		case "invalidlogin":
			return "", errors.New("اسم المستخدم أو كلمة المرور غير صحيحة")
		case "enablewsdescription", "servicenotavailable":
			return "", errors.New("الجامعة لم تفعّل خدمة تطبيق المودل. استخدم خيار (الدخول بالتوكن)")
		case "sitepolicynotagreed":
			return "", errors.New("يجب الموافقة على سياسة الموقع أولاً من المتصفح ثم أعد المحاولة")
		}
		if r.Error != "" {
			return "", errors.New(r.Error)
		}
		return "", errors.New("فشل تسجيل الدخول")
	}
	return r.Token, nil
}

func NewMoodle(base, token string) *Moodle {
	return &Moodle{Base: base, Token: token, client: httpClient}
}

// call runs a web service function. params may contain keys like "courseids[0]".
func (m *Moodle) call(fn string, params map[string]string, out any) error {
	form := url.Values{"wstoken": {m.Token}, "wsfunction": {fn}, "moodlewsrestformat": {"json"}}
	for k, v := range params {
		form.Set(k, v)
	}
	resp, err := m.client.PostForm(m.Base+"/webservice/rest/server.php", form)
	if err != nil {
		return fmt.Errorf("تعذر الاتصال بالمودل: %v", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if len(body) > 0 && body[0] == '{' {
		var e struct {
			Exception string `json:"exception"`
			ErrorCode string `json:"errorcode"`
			Message   string `json:"message"`
		}
		if json.Unmarshal(body, &e) == nil && e.Exception != "" {
			if e.ErrorCode == "invalidtoken" {
				return &MoodleError{Code: e.ErrorCode, Message: "انتهت صلاحية الدخول، سجّل الدخول من جديد في الإعدادات"}
			}
			return &MoodleError{Code: e.ErrorCode, Message: e.Message}
		}
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("رد غير متوقع من المودل (%s): %v", fn, err)
	}
	return nil
}

type SiteInfo struct {
	SiteName string `json:"sitename"`
	FullName string `json:"fullname"`
	UserID   int    `json:"userid"`
	Username string `json:"username"`
}

func (m *Moodle) SiteInfo() (SiteInfo, error) {
	var s SiteInfo
	err := m.call("core_webservice_get_site_info", nil, &s)
	return s, err
}

type Course struct {
	ID        int    `json:"id"`
	ShortName string `json:"shortname"`
	FullName  string `json:"fullname"`
	Hidden    bool   `json:"hidden"`
	EndDate   int64  `json:"enddate"`
}

func (m *Moodle) Courses(userID int) ([]Course, error) {
	var cs []Course
	err := m.call("core_enrol_get_users_courses", map[string]string{"userid": strconv.Itoa(userID)}, &cs)
	return cs, err
}

type ContentFile struct {
	Type         string `json:"type"`
	FileName     string `json:"filename"`
	FilePath     string `json:"filepath"`
	FileURL      string `json:"fileurl"`
	FileSize     int64  `json:"filesize"`
	TimeCreated  int64  `json:"timecreated"`
	TimeModified int64  `json:"timemodified"`
}

type Module struct {
	ID          int           `json:"id"`
	Name        string        `json:"name"`
	ModName     string        `json:"modname"`
	URL         string        `json:"url"`
	Description string        `json:"description"` // HTML shown on the course page (label text, intros)
	Contents    []ContentFile `json:"contents"`
}

type Section struct {
	ID      int      `json:"id"`
	Name    string   `json:"name"`
	Section int      `json:"section"`
	Summary string   `json:"summary"`
	Modules []Module `json:"modules"`
}

func (m *Moodle) CourseContents(courseID int) ([]Section, error) {
	var s []Section
	err := m.call("core_course_get_contents", map[string]string{"courseid": strconv.Itoa(courseID)}, &s)
	return s, err
}

type Assignment struct {
	ID               int           `json:"id"`
	CmID             int           `json:"cmid"`
	Course           int           `json:"course"`
	Name             string        `json:"name"`
	DueDate          int64         `json:"duedate"`
	CutoffDate       int64         `json:"cutoffdate"`
	Intro            string        `json:"intro"`
	IntroAttachments []ContentFile `json:"introattachments"`
	IntroFiles       []ContentFile `json:"introfiles"`
}

func (m *Moodle) Assignments(courseIDs []int) (map[int][]Assignment, error) {
	params := map[string]string{}
	for i, id := range courseIDs {
		params[fmt.Sprintf("courseids[%d]", i)] = strconv.Itoa(id)
	}
	var r struct {
		Courses []struct {
			ID          int          `json:"id"`
			Assignments []Assignment `json:"assignments"`
		} `json:"courses"`
	}
	if err := m.call("mod_assign_get_assignments", params, &r); err != nil {
		return nil, err
	}
	out := map[int][]Assignment{}
	for _, c := range r.Courses {
		out[c.ID] = c.Assignments
	}
	return out, nil
}

// SubmissionStatus returns "submitted", "draft", "new" or "" if unknown.
func (m *Moodle) SubmissionStatus(assignID int) (string, error) {
	var r struct {
		LastAttempt struct {
			Submission struct {
				Status string `json:"status"`
			} `json:"submission"`
			TeamSubmission struct {
				Status string `json:"status"`
			} `json:"teamsubmission"`
		} `json:"lastattempt"`
	}
	if err := m.call("mod_assign_get_submission_status", map[string]string{"assignid": strconv.Itoa(assignID)}, &r); err != nil {
		return "", err
	}
	s := r.LastAttempt.Submission.Status
	if s == "" {
		s = r.LastAttempt.TeamSubmission.Status
	}
	return s, nil
}

type MNotification struct {
	ID          int    `json:"id"`
	Subject     string `json:"subject"`
	Small       string `json:"smallmessage"`
	FullHTML    string `json:"fullmessagehtml"`
	Full        string `json:"fullmessage"`
	TimeCreated int64  `json:"timecreated"`
	Read        bool   `json:"read"`
	ContextURL  string `json:"contexturl"`
	Component   string `json:"component"`
}

func (m *Moodle) Notifications(userID int) ([]MNotification, error) {
	var r struct {
		Notifications []MNotification `json:"notifications"`
	}
	err := m.call("message_popup_get_popup_notifications", map[string]string{
		"useridto": strconv.Itoa(userID), "limit": "50", "offset": "0", "newestfirst": "1",
	}, &r)
	if err == nil {
		return r.Notifications, nil
	}
	// fallback for sites where the popup plugin is not available
	var r2 struct {
		Messages []struct {
			ID          int    `json:"id"`
			Subject     string `json:"subject"`
			Small       string `json:"smallmessage"`
			Full        string `json:"fullmessage"`
			TimeCreated int64  `json:"timecreated"`
			ContextURL  string `json:"contexturl"`
			Component   string `json:"component"`
			TimeRead    int64  `json:"timeread"`
		} `json:"messages"`
	}
	err2 := m.call("core_message_get_messages", map[string]string{
		"useridto": strconv.Itoa(userID), "type": "notifications", "read": "2", "newestfirst": "1", "limitnum": "50",
	}, &r2)
	if err2 != nil {
		return nil, err
	}
	var out []MNotification
	for _, x := range r2.Messages {
		out = append(out, MNotification{ID: x.ID, Subject: x.Subject, Small: x.Small, Full: x.Full,
			TimeCreated: x.TimeCreated, ContextURL: x.ContextURL, Component: x.Component, Read: x.TimeRead > 0})
	}
	return out, nil
}

type CalEvent struct {
	ID         int    `json:"id"`
	Name       string `json:"name"`
	TimeSort   int64  `json:"timesort"`
	ModuleName string `json:"modulename"`
	URL        string `json:"url"`
	Course     struct {
		ID       int    `json:"id"`
		FullName string `json:"fullname"`
	} `json:"course"`
}

func (m *Moodle) UpcomingEvents() ([]CalEvent, error) {
	var r struct {
		Events []CalEvent `json:"events"`
	}
	err := m.call("core_calendar_get_action_events_by_timesort", map[string]string{
		"timesortfrom": strconv.FormatInt(time.Now().Add(-24*time.Hour).Unix(), 10), "limitnum": "50",
	}, &r)
	return r.Events, err
}

// authURL adds the token to a Moodle file URL (pluginfile.php needs its webservice variant).
func (m *Moodle) authURL(fileURL string) string {
	u := fileURL
	if strings.Contains(u, "?") {
		u += "&token=" + url.QueryEscape(m.Token)
	} else {
		u += "?token=" + url.QueryEscape(m.Token)
	}
	if strings.Contains(u, "/pluginfile.php") && !strings.Contains(u, "/webservice/pluginfile.php") {
		u = strings.Replace(u, "/pluginfile.php", "/webservice/pluginfile.php", 1)
	}
	return u
}

// sameSite reports whether u is on the Moodle site, so the token is never sent anywhere else.
func (m *Moodle) sameSite(u string) bool {
	a, err1 := url.Parse(u)
	b, err2 := url.Parse(m.Base)
	return err1 == nil && err2 == nil && a.Host != "" && strings.EqualFold(a.Host, b.Host)
}

// Download saves a Moodle file to dest (via a temp file so partial downloads never appear).
func (m *Moodle) Download(fileURL, dest string) error {
	u := m.authURL(fileURL)
	c := &http.Client{Timeout: 30 * time.Minute}
	resp, err := c.Get(u)
	if err != nil {
		return errors.New(redact(err.Error())) // never leak the token into logs
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	ct := resp.Header.Get("Content-Type")
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	tmp := dest + ".part"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	n, err := io.Copy(f, resp.Body)
	f.Close()
	if err != nil {
		os.Remove(tmp)
		return err
	}
	// Moodle returns a JSON error with 200 for bad tokens
	if strings.Contains(ct, "application/json") && n < 2000 {
		b, _ := os.ReadFile(tmp)
		if strings.Contains(string(b), "\"error\"") || strings.Contains(string(b), "\"exception\"") {
			os.Remove(tmp)
			return errors.New("رفض المودل تنزيل الملف")
		}
	}
	return os.Rename(tmp, dest)
}
