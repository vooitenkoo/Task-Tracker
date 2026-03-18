package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"gosystem/internal/store"
)

type Handler struct {
	tasks       *store.TaskStore
	projects    *store.ProjectStore
	comments    *store.CommentStore
	users       *store.UserStore
	assignees   *store.AssigneeStore
	members     *store.ProjectMemberStore
	checklists  *store.ChecklistStore
	activity    *store.ActivityStore
	tags        *store.TagStore
	log         *log.Logger
}

func NewHandler(
	tasks *store.TaskStore,
	projects *store.ProjectStore,
	comments *store.CommentStore,
	users *store.UserStore,
	assignees *store.AssigneeStore,
	members *store.ProjectMemberStore,
	checklists *store.ChecklistStore,
	activity *store.ActivityStore,
	tags *store.TagStore,
	log *log.Logger,
) http.Handler {
	h := &Handler{
		tasks:      tasks,
		projects:   projects,
		comments:   comments,
		users:      users,
		assignees:  assignees,
		members:    members,
		checklists: checklists,
		activity:   activity,
		tags:       tags,
		log:        log,
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/health", h.health)
	mux.HandleFunc("/tasks/overdue", h.overdueTasks)
	mux.HandleFunc("/tasks/due-soon", h.dueSoonTasks)
	mux.HandleFunc("/tasks/bulk/status", h.bulkStatus)
	mux.HandleFunc("/tasks", h.tasks)
	mux.HandleFunc("/tasks/", h.taskByID)
	mux.HandleFunc("/projects", h.projects)
	mux.HandleFunc("/projects/", h.projectByID)
	mux.HandleFunc("/users", h.users)
	mux.HandleFunc("/users/", h.userByID)
	mux.HandleFunc("/tags", h.tagsList)
	mux.HandleFunc("/stats", h.stats)
	return withMiddleware(mux, log)
}

func (h *Handler) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handler) tasks(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		h.createTask(w, r)
	case http.MethodGet:
		h.listTasks(w, r)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (h *Handler) taskByID(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/tasks/")
	if path == "" {
		writeError(w, http.StatusNotFound, "not found")
		return
	}

	parts := strings.Split(path, "/")
	id := parts[0]
	if id == "" {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}

	if len(parts) == 1 && r.Method == http.MethodGet {
		h.getTask(w, r, id)
		return
	}

	if len(parts) == 1 && r.Method == http.MethodPatch {
		h.updateTask(w, r, id)
		return
	}

	if len(parts) == 1 && r.Method == http.MethodDelete {
		h.deleteTask(w, r, id)
		return
	}

	if len(parts) == 2 && parts[1] == "status" && r.Method == http.MethodPatch {
		h.updateStatus(w, r, id)
		return
	}

	if len(parts) == 2 && parts[1] == "comments" && r.Method == http.MethodPost {
		h.createComment(w, r, id)
		return
	}

	if len(parts) == 2 && parts[1] == "comments" && r.Method == http.MethodGet {
		h.listComments(w, r, id)
		return
	}

	if len(parts) == 2 && parts[1] == "assignees" && r.Method == http.MethodPost {
		h.addAssignee(w, r, id)
		return
	}

	if len(parts) == 2 && parts[1] == "assignees" && r.Method == http.MethodGet {
		h.listAssignees(w, r, id)
		return
	}

	if len(parts) == 3 && parts[1] == "assignees" && r.Method == http.MethodDelete {
		h.removeAssignee(w, r, id, parts[2])
		return
	}

	if len(parts) == 2 && parts[1] == "checklist" && r.Method == http.MethodPost {
		h.addChecklistItem(w, r, id)
		return
	}

	if len(parts) == 3 && parts[1] == "checklist" && r.Method == http.MethodPatch {
		h.updateChecklistItem(w, r, id, parts[2])
		return
	}

	if len(parts) == 3 && parts[1] == "checklist" && r.Method == http.MethodDelete {
		h.deleteChecklistItem(w, r, id, parts[2])
		return
	}

	if len(parts) == 2 && parts[1] == "checklist" && r.Method == http.MethodGet {
		h.listChecklistItems(w, r, id)
		return
	}

	if len(parts) == 2 && parts[1] == "activity" && r.Method == http.MethodGet {
		h.listActivity(w, r, id)
		return
	}

	writeError(w, http.StatusNotFound, "not found")
}

func (h *Handler) createTask(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Title       string     `json:"title"`
		Description string     `json:"description"`
		Priority    int        `json:"priority"`
		DueDate     *time.Time `json:"due_date"`
		ProjectID   *string    `json:"project_id"`
		Tags        []string   `json:"tags"`
	}
	if err := readJSON(r, &req); err != nil || strings.TrimSpace(req.Title) == "" {
		writeError(w, http.StatusBadRequest, "invalid payload")
		return
	}

	priority := req.Priority
	if priority == 0 {
		priority = 3
	}
	if priority < 1 || priority > 5 {
		writeError(w, http.StatusBadRequest, "priority must be 1..5")
		return
	}

	input := store.CreateTaskInput{
		Title:       strings.TrimSpace(req.Title),
		Description: strings.TrimSpace(req.Description),
		Priority:    priority,
		DueDate:     req.DueDate,
		ProjectID:   req.ProjectID,
		Tags:        req.Tags,
	}
	task, err := h.tasks.Create(r.Context(), input)
	if err != nil {
		h.log.Printf("create task failed: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to create task")
		return
	}
	h.logActivity(r.Context(), task.ID, "task_created", "task created")

	writeJSON(w, http.StatusCreated, task)
}

func (h *Handler) listTasks(w http.ResponseWriter, r *http.Request) {
	limit := clampInt(queryInt(r, "limit", 50), 1, 200)
	offset := clampInt(queryInt(r, "offset", 0), 0, 10000)
	status := strings.TrimSpace(r.URL.Query().Get("status"))
	search := strings.TrimSpace(r.URL.Query().Get("search"))
	tag := strings.TrimSpace(r.URL.Query().Get("tag"))
	projectID := strings.TrimSpace(r.URL.Query().Get("project_id"))
	priority := queryIntPointer(r, "priority")

	if status != "" && !isAllowedStatus(status) {
		writeError(w, http.StatusBadRequest, "invalid status")
		return
	}
	if priority != nil && (*priority < 1 || *priority > 5) {
		writeError(w, http.StatusBadRequest, "priority must be 1..5")
		return
	}

	filters := store.ListFilters{
		Status:    status,
		Priority:  priority,
		ProjectID: projectID,
		Search:    search,
		Tag:       tag,
	}

	tasks, err := h.tasks.List(r.Context(), filters, limit, offset)
	if err != nil {
		h.log.Printf("list tasks failed: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to list tasks")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"items":  tasks,
		"limit":  limit,
		"offset": offset,
	})
}

func (h *Handler) getTask(w http.ResponseWriter, r *http.Request, id string) {
	task, err := h.tasks.Get(r.Context(), id)
	if err != nil {
		if err == store.ErrNotFound {
			writeError(w, http.StatusNotFound, "task not found")
			return
		}
		h.log.Printf("get task failed: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to fetch task")
		return
	}
	writeJSON(w, http.StatusOK, task)
}

func (h *Handler) updateStatus(w http.ResponseWriter, r *http.Request, id string) {
	var req struct {
		Status string `json:"status"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid payload")
		return
	}

	status := strings.TrimSpace(req.Status)
	if !isAllowedStatus(status) {
		writeError(w, http.StatusBadRequest, "invalid status")
		return
	}

	task, err := h.tasks.UpdateStatus(r.Context(), id, status)
	if err != nil {
		if err == store.ErrNotFound {
			writeError(w, http.StatusNotFound, "task not found")
			return
		}
		h.log.Printf("update status failed: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to update status")
		return
	}
	h.logActivity(r.Context(), task.ID, "status_changed", fmt.Sprintf("status -> %s", status))
	writeJSON(w, http.StatusOK, task)
}

func (h *Handler) updateTask(w http.ResponseWriter, r *http.Request, id string) {
	var req struct {
		Title         *string    `json:"title"`
		Description   *string    `json:"description"`
		Priority      *int       `json:"priority"`
		DueDate       *time.Time `json:"due_date"`
		ProjectID     *string    `json:"project_id"`
		Tags          *[]string  `json:"tags"`
		ClearDueDate  bool       `json:"clear_due_date"`
		ClearProject  bool       `json:"clear_project_id"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid payload")
		return
	}

	if req.Title != nil && strings.TrimSpace(*req.Title) == "" {
		writeError(w, http.StatusBadRequest, "title cannot be empty")
		return
	}
	if req.Priority != nil && (*req.Priority < 1 || *req.Priority > 5) {
		writeError(w, http.StatusBadRequest, "priority must be 1..5")
		return
	}

	if req.Title != nil {
		value := strings.TrimSpace(*req.Title)
		req.Title = &value
	}
	if req.Description != nil {
		value := strings.TrimSpace(*req.Description)
		req.Description = &value
	}

	input := store.UpdateTaskInput{
		Title:        req.Title,
		Description:  req.Description,
		Priority:     req.Priority,
		DueDate:      req.DueDate,
		ProjectID:    req.ProjectID,
		Tags:         req.Tags,
		ClearDueDate: req.ClearDueDate,
		ClearProject: req.ClearProject,
	}

	task, err := h.tasks.Update(r.Context(), id, input)
	if err != nil {
		if err == store.ErrNotFound {
			writeError(w, http.StatusNotFound, "task not found")
			return
		}
		h.log.Printf("update task failed: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to update task")
		return
	}
	h.logActivity(r.Context(), task.ID, "task_updated", "task updated")
	writeJSON(w, http.StatusOK, task)
}

func (h *Handler) createComment(w http.ResponseWriter, r *http.Request, taskID string) {
	var req struct {
		Author string `json:"author"`
		Body   string `json:"body"`
	}
	if err := readJSON(r, &req); err != nil || strings.TrimSpace(req.Body) == "" {
		writeError(w, http.StatusBadRequest, "invalid payload")
		return
	}
	author := strings.TrimSpace(req.Author)
	if author == "" {
		author = "anonymous"
	}

	if err := h.comments.EnsureTaskExists(r.Context(), taskID); err != nil {
		if err == store.ErrNotFound {
			writeError(w, http.StatusNotFound, "task not found")
			return
		}
		h.log.Printf("comment check failed: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to add comment")
		return
	}

	comment, err := h.comments.Create(r.Context(), taskID, author, strings.TrimSpace(req.Body))
	if err != nil {
		h.log.Printf("create comment failed: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to add comment")
		return
	}
	h.logActivity(r.Context(), taskID, "comment_added", fmt.Sprintf("comment by %s", author))
	writeJSON(w, http.StatusCreated, comment)
}

func (h *Handler) listComments(w http.ResponseWriter, r *http.Request, taskID string) {
	limit := clampInt(queryInt(r, "limit", 50), 1, 200)
	offset := clampInt(queryInt(r, "offset", 0), 0, 10000)

	if err := h.comments.EnsureTaskExists(r.Context(), taskID); err != nil {
		if err == store.ErrNotFound {
			writeError(w, http.StatusNotFound, "task not found")
			return
		}
		h.log.Printf("comment check failed: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to list comments")
		return
	}

	comments, err := h.comments.ListByTask(r.Context(), taskID, limit, offset)
	if err != nil {
		h.log.Printf("list comments failed: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to list comments")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"items":  comments,
		"limit":  limit,
		"offset": offset,
	})
}

func (h *Handler) deleteTask(w http.ResponseWriter, r *http.Request, id string) {
	if err := h.tasks.Delete(r.Context(), id); err != nil {
		if err == store.ErrNotFound {
			writeError(w, http.StatusNotFound, "task not found")
			return
		}
		h.log.Printf("delete task failed: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to delete task")
		return
	}
	h.logActivity(r.Context(), id, "task_deleted", "task deleted")
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func (h *Handler) addAssignee(w http.ResponseWriter, r *http.Request, taskID string) {
	var req struct {
		UserID string `json:"user_id"`
	}
	if err := readJSON(r, &req); err != nil || strings.TrimSpace(req.UserID) == "" {
		writeError(w, http.StatusBadRequest, "invalid payload")
		return
	}
	if _, err := h.tasks.Get(r.Context(), taskID); err != nil {
		if err == store.ErrNotFound {
			writeError(w, http.StatusNotFound, "task not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to add assignee")
		return
	}
	if _, err := h.users.Get(r.Context(), req.UserID); err != nil {
		if err == store.ErrNotFound {
			writeError(w, http.StatusNotFound, "user not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to add assignee")
		return
	}
	if err := h.assignees.Add(r.Context(), taskID, req.UserID); err != nil {
		h.log.Printf("add assignee failed: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to add assignee")
		return
	}
	h.logActivity(r.Context(), taskID, "assignee_added", fmt.Sprintf("assignee %s", req.UserID))
	writeJSON(w, http.StatusCreated, map[string]string{"status": "added"})
}

func (h *Handler) listAssignees(w http.ResponseWriter, r *http.Request, taskID string) {
	assignees, err := h.assignees.List(r.Context(), taskID)
	if err != nil {
		h.log.Printf("list assignees failed: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to list assignees")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": assignees})
}

func (h *Handler) removeAssignee(w http.ResponseWriter, r *http.Request, taskID, userID string) {
	if err := h.assignees.Remove(r.Context(), taskID, userID); err != nil {
		h.log.Printf("remove assignee failed: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to remove assignee")
		return
	}
	h.logActivity(r.Context(), taskID, "assignee_removed", fmt.Sprintf("assignee %s", userID))
	writeJSON(w, http.StatusOK, map[string]string{"status": "removed"})
}

func (h *Handler) addChecklistItem(w http.ResponseWriter, r *http.Request, taskID string) {
	var req struct {
		Title string `json:"title"`
	}
	if err := readJSON(r, &req); err != nil || strings.TrimSpace(req.Title) == "" {
		writeError(w, http.StatusBadRequest, "invalid payload")
		return
	}
	item, err := h.checklists.Create(r.Context(), taskID, strings.TrimSpace(req.Title))
	if err != nil {
		h.log.Printf("create checklist item failed: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to create checklist item")
		return
	}
	h.logActivity(r.Context(), taskID, "checklist_added", fmt.Sprintf("checklist item %s", item.ID))
	writeJSON(w, http.StatusCreated, item)
}

func (h *Handler) updateChecklistItem(w http.ResponseWriter, r *http.Request, taskID, itemID string) {
	var req struct {
		Title *string `json:"title"`
		Done  *bool   `json:"done"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid payload")
		return
	}
	if req.Title != nil && strings.TrimSpace(*req.Title) == "" {
		writeError(w, http.StatusBadRequest, "title cannot be empty")
		return
	}
	if req.Title != nil {
		value := strings.TrimSpace(*req.Title)
		req.Title = &value
	}
	item, err := h.checklists.Update(r.Context(), itemID, req.Title, req.Done)
	if err != nil {
		if err == store.ErrNotFound {
			writeError(w, http.StatusNotFound, "checklist item not found")
			return
		}
		h.log.Printf("update checklist item failed: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to update checklist item")
		return
	}
	h.logActivity(r.Context(), taskID, "checklist_updated", fmt.Sprintf("checklist item %s", item.ID))
	writeJSON(w, http.StatusOK, item)
}

func (h *Handler) deleteChecklistItem(w http.ResponseWriter, r *http.Request, taskID, itemID string) {
	if err := h.checklists.Delete(r.Context(), itemID); err != nil {
		h.log.Printf("delete checklist item failed: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to delete checklist item")
		return
	}
	h.logActivity(r.Context(), taskID, "checklist_deleted", fmt.Sprintf("checklist item %s", itemID))
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func (h *Handler) listChecklistItems(w http.ResponseWriter, r *http.Request, taskID string) {
	items, err := h.checklists.ListByTask(r.Context(), taskID)
	if err != nil {
		h.log.Printf("list checklist items failed: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to list checklist items")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (h *Handler) listActivity(w http.ResponseWriter, r *http.Request, taskID string) {
	limit := clampInt(queryInt(r, "limit", 50), 1, 200)
	offset := clampInt(queryInt(r, "offset", 0), 0, 10000)
	entries, err := h.activity.ListByTask(r.Context(), taskID, limit, offset)
	if err != nil {
		h.log.Printf("list activity failed: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to list activity")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"items":  entries,
		"limit":  limit,
		"offset": offset,
	})
}

func (h *Handler) projects(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		h.createProject(w, r)
	case http.MethodGet:
		h.listProjects(w, r)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (h *Handler) users(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		h.createUser(w, r)
	case http.MethodGet:
		h.listUsers(w, r)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (h *Handler) userByID(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/users/")
	if path == "" {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	h.getUser(w, r, path)
}

func (h *Handler) projectByID(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/projects/")
	if path == "" {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	parts := strings.Split(path, "/")
	projectID := parts[0]
	if projectID == "" {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}

	if len(parts) == 1 && r.Method == http.MethodGet {
		h.getProject(w, r, projectID)
		return
	}

	if len(parts) == 2 && parts[1] == "members" && r.Method == http.MethodPost {
		h.addProjectMember(w, r, projectID)
		return
	}

	if len(parts) == 2 && parts[1] == "members" && r.Method == http.MethodGet {
		h.listProjectMembers(w, r, projectID)
		return
	}

	if len(parts) == 2 && parts[1] == "stats" && r.Method == http.MethodGet {
		h.projectStats(w, r, projectID)
		return
	}

	writeError(w, http.StatusNotFound, "not found")
}

func (h *Handler) createProject(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name string `json:"name"`
	}
	if err := readJSON(r, &req); err != nil || strings.TrimSpace(req.Name) == "" {
		writeError(w, http.StatusBadRequest, "invalid payload")
		return
	}
	project, err := h.projects.Create(r.Context(), strings.TrimSpace(req.Name))
	if err != nil {
		h.log.Printf("create project failed: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to create project")
		return
	}
	writeJSON(w, http.StatusCreated, project)
}

func (h *Handler) createUser(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name  string `json:"name"`
		Email string `json:"email"`
	}
	if err := readJSON(r, &req); err != nil || strings.TrimSpace(req.Name) == "" || strings.TrimSpace(req.Email) == "" {
		writeError(w, http.StatusBadRequest, "invalid payload")
		return
	}
	user, err := h.users.Create(r.Context(), strings.TrimSpace(req.Name), strings.TrimSpace(req.Email))
	if err != nil {
		h.log.Printf("create user failed: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to create user")
		return
	}
	writeJSON(w, http.StatusCreated, user)
}

func (h *Handler) listProjects(w http.ResponseWriter, r *http.Request) {
	limit := clampInt(queryInt(r, "limit", 50), 1, 200)
	offset := clampInt(queryInt(r, "offset", 0), 0, 10000)
	projects, err := h.projects.List(r.Context(), limit, offset)
	if err != nil {
		h.log.Printf("list projects failed: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to list projects")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"items":  projects,
		"limit":  limit,
		"offset": offset,
	})
}

func (h *Handler) listUsers(w http.ResponseWriter, r *http.Request) {
	limit := clampInt(queryInt(r, "limit", 50), 1, 200)
	offset := clampInt(queryInt(r, "offset", 0), 0, 10000)
	users, err := h.users.List(r.Context(), limit, offset)
	if err != nil {
		h.log.Printf("list users failed: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to list users")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"items":  users,
		"limit":  limit,
		"offset": offset,
	})
}

func (h *Handler) getProject(w http.ResponseWriter, r *http.Request, id string) {
	project, err := h.projects.Get(r.Context(), id)
	if err != nil {
		if err == store.ErrNotFound {
			writeError(w, http.StatusNotFound, "project not found")
			return
		}
		h.log.Printf("get project failed: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to fetch project")
		return
	}
	writeJSON(w, http.StatusOK, project)
}

func (h *Handler) getUser(w http.ResponseWriter, r *http.Request, id string) {
	user, err := h.users.Get(r.Context(), id)
	if err != nil {
		if err == store.ErrNotFound {
			writeError(w, http.StatusNotFound, "user not found")
			return
		}
		h.log.Printf("get user failed: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to fetch user")
		return
	}
	writeJSON(w, http.StatusOK, user)
}

func (h *Handler) addProjectMember(w http.ResponseWriter, r *http.Request, projectID string) {
	var req struct {
		UserID string `json:"user_id"`
		Role   string `json:"role"`
	}
	if err := readJSON(r, &req); err != nil || strings.TrimSpace(req.UserID) == "" {
		writeError(w, http.StatusBadRequest, "invalid payload")
		return
	}
	role := strings.TrimSpace(req.Role)
	if role == "" {
		role = "member"
	}

	if _, err := h.projects.Get(r.Context(), projectID); err != nil {
		if err == store.ErrNotFound {
			writeError(w, http.StatusNotFound, "project not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to add member")
		return
	}
	if _, err := h.users.Get(r.Context(), req.UserID); err != nil {
		if err == store.ErrNotFound {
			writeError(w, http.StatusNotFound, "user not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to add member")
		return
	}

	if err := h.members.Add(r.Context(), projectID, req.UserID, role); err != nil {
		h.log.Printf("add project member failed: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to add member")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"status": "added"})
}

func (h *Handler) listProjectMembers(w http.ResponseWriter, r *http.Request, projectID string) {
	members, err := h.members.List(r.Context(), projectID)
	if err != nil {
		h.log.Printf("list project members failed: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to list members")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": members})
}

func (h *Handler) projectStats(w http.ResponseWriter, r *http.Request, projectID string) {
	stats, err := h.tasks.ProjectStats(r.Context(), projectID)
	if err != nil {
		if err == store.ErrNotFound {
			writeError(w, http.StatusNotFound, "project not found")
			return
		}
		h.log.Printf("project stats failed: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to fetch project stats")
		return
	}
	writeJSON(w, http.StatusOK, stats)
}

func (h *Handler) stats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	statusCounts, overdue, err := h.tasks.Stats(r.Context())
	if err != nil {
		h.log.Printf("stats failed: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to fetch stats")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"by_status": statusCounts,
		"overdue":   overdue,
	})
}

func (h *Handler) logActivity(ctx context.Context, taskID, entryType, message string) {
	if h.activity == nil {
		return
	}
	if err := h.activity.Add(ctx, taskID, entryType, message); err != nil {
		h.log.Printf("activity log failed: %v", err)
	}
}

func (h *Handler) overdueTasks(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	limit := clampInt(queryInt(r, "limit", 50), 1, 200)
	offset := clampInt(queryInt(r, "offset", 0), 0, 10000)
	tasks, err := h.tasks.ListOverdue(r.Context(), limit, offset)
	if err != nil {
		h.log.Printf("overdue tasks failed: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to list overdue tasks")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"items":  tasks,
		"limit":  limit,
		"offset": offset,
	})
}

func (h *Handler) dueSoonTasks(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	days := clampInt(queryInt(r, "days", 3), 1, 30)
	limit := clampInt(queryInt(r, "limit", 50), 1, 200)
	offset := clampInt(queryInt(r, "offset", 0), 0, 10000)
	tasks, err := h.tasks.ListDueSoon(r.Context(), days, limit, offset)
	if err != nil {
		h.log.Printf("due soon tasks failed: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to list due soon tasks")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"items":  tasks,
		"limit":  limit,
		"offset": offset,
		"days":   days,
	})
}

func (h *Handler) bulkStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var req struct {
		IDs    []string `json:"ids"`
		Status string   `json:"status"`
	}
	if err := readJSON(r, &req); err != nil || len(req.IDs) == 0 {
		writeError(w, http.StatusBadRequest, "invalid payload")
		return
	}
	status := strings.TrimSpace(req.Status)
	if !isAllowedStatus(status) {
		writeError(w, http.StatusBadRequest, "invalid status")
		return
	}
	updated, err := h.tasks.BulkUpdateStatus(r.Context(), req.IDs, status)
	if err != nil {
		h.log.Printf("bulk status failed: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to update status")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"updated": updated})
}

func (h *Handler) tagsList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	limit := clampInt(queryInt(r, "limit", 100), 1, 500)
	offset := clampInt(queryInt(r, "offset", 0), 0, 10000)
	tags, err := h.tags.List(r.Context(), limit, offset)
	if err != nil {
		h.log.Printf("list tags failed: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to list tags")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"items":  tags,
		"limit":  limit,
		"offset": offset,
	})
}

func isAllowedStatus(status string) bool {
	switch status {
	case "open", "in_progress", "done":
		return true
	default:
		return false
	}
}

func readJSON(r *http.Request, out any) error {
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	return decoder.Decode(out)
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (sw *statusWriter) WriteHeader(code int) {
	sw.status = code
	sw.ResponseWriter.WriteHeader(code)
}

func withMiddleware(next http.Handler, log *log.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}

		defer func() {
			if recovered := recover(); recovered != nil {
				log.Printf("panic recovered: %v", recovered)
				writeError(sw, http.StatusInternalServerError, "internal error")
			}
		}()

		next.ServeHTTP(sw, r)
		log.Printf("%s %s %d %dms", r.Method, r.URL.Path, sw.status, time.Since(start).Milliseconds())
	})
}

func queryInt(r *http.Request, key string, fallback int) int {
	value := r.URL.Query().Get(key)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func queryIntPointer(r *http.Request, key string) *int {
	value := r.URL.Query().Get(key)
	if value == "" {
		return nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return nil
	}
	return &parsed
}

func clampInt(value, min, max int) int {
	if value < min {
		return min
	}
	if value > max {
		return max
	}
	return value
}
