package server

import (
	"fmt"
	"log"
	"time"
)

// ScheduledJob represents a sync job that needs to be executed on schedule
type ScheduledJob struct {
	ID               int       `json:"id"`
	Name             string    `json:"name"`
	SourceAgentID    string    `json:"source_agent_id"`
	TargetAgentID    string    `json:"target_agent_id"`
	ScheduleType     string    `json:"schedule_type"`
	Status           string    `json:"status"`
	LastScheduledRun *time.Time `json:"last_scheduled_run"`
	NextScheduledRun *time.Time `json:"next_scheduled_run"`
}

// JobScheduler handles scheduled sync jobs
type JobScheduler struct {
	server   *SyncToolServer
	stopChan chan struct{}
	running  bool
}

// NewJobScheduler creates a new job scheduler
func NewJobScheduler(server *SyncToolServer) *JobScheduler {
	return &JobScheduler{
		server:   server,
		stopChan: make(chan struct{}),
		running:  false,
	}
}

// Start begins the scheduler with a ticker that runs every minute
func (js *JobScheduler) Start() {
	if js.running {
		log.Println("⚠️ Job scheduler is already running")
		return
	}

	js.running = true
	log.Println("🕒 Starting job scheduler...")

	// Initialize next run times for jobs that don't have them
	if err := js.initializeSchedules(); err != nil {
		log.Printf("❌ Failed to initialize schedules: %v", err)
	}

	// Create ticker that runs every minute
	ticker := time.NewTicker(1 * time.Minute)
	
	go func() {
		defer ticker.Stop()
		log.Println("✅ Job scheduler started - checking every minute")

		for {
			select {
			case <-ticker.C:
				if err := js.processScheduledJobs(); err != nil {
					log.Printf("❌ Error processing scheduled jobs: %v", err)
				}
			case <-js.stopChan:
				log.Println("🛑 Job scheduler stopping...")
				js.running = false
				return
			}
		}
	}()
}

// Stop stops the scheduler
func (js *JobScheduler) Stop() {
	if !js.running {
		return
	}
	
	log.Println("🛑 Stopping job scheduler...")
	close(js.stopChan)
}

// initializeSchedules sets up next_scheduled_run for jobs that don't have it set
func (js *JobScheduler) initializeSchedules() error {
	log.Println("🔧 Initializing schedules for jobs without next_scheduled_run...")

	// Get jobs that need initialization
	rows, err := js.server.db.Query(`
		SELECT id, schedule_type, last_scheduled_run
		FROM sync_jobs 
		WHERE schedule_type != 'continuous' 
		AND status = 'active'
		AND next_scheduled_run IS NULL
	`)
	if err != nil {
		return fmt.Errorf("failed to query uninitialized jobs: %w", err)
	}
	defer rows.Close()

	var updateCount int
	for rows.Next() {
		var jobID int
		var scheduleType string
		var lastRun *time.Time

		if err := rows.Scan(&jobID, &scheduleType, &lastRun); err != nil {
			log.Printf("❌ Failed to scan job row: %v", err)
			continue
		}

		// Calculate next run time
		nextRun := js.calculateNextRun(scheduleType, lastRun)
		
		// Update the job
		_, err := js.server.db.Exec(`
			UPDATE sync_jobs 
			SET next_scheduled_run = $1, updated_at = $2
			WHERE id = $3
		`, nextRun, time.Now(), jobID)
		
		if err != nil {
			log.Printf("❌ Failed to initialize schedule for job %d: %v", jobID, err)
			continue
		}

		updateCount++
		log.Printf("🕒 Initialized schedule for job %d (%s): next run at %s", 
			jobID, scheduleType, nextRun.Format("2006-01-02 15:04:05"))
	}

	log.Printf("✅ Initialized %d job schedules", updateCount)
	return nil
}

// processScheduledJobs finds and executes jobs that are ready to run
func (js *JobScheduler) processScheduledJobs() error {
	now := time.Now()
	
	// Get jobs that are ready to run
	jobs, err := js.getJobsToRun(now)
	if err != nil {
		return fmt.Errorf("failed to get jobs to run: %w", err)
	}

	if len(jobs) == 0 {
		// Only log every 10 minutes to avoid spam
		if now.Minute()%10 == 0 {
			log.Printf("🕒 No scheduled jobs to run at %s", now.Format("15:04:05"))
		}
		return nil
	}

	log.Printf("🚀 Processing %d scheduled jobs at %s", len(jobs), now.Format("15:04:05"))

	// Process each job
	for _, job := range jobs {
		if err := js.executeJob(job); err != nil {
			log.Printf("❌ Failed to execute job %d (%s): %v", job.ID, job.Name, err)
		} else {
			log.Printf("✅ Successfully executed job %d (%s)", job.ID, job.Name)
		}
	}

	return nil
}

// getJobsToRun queries the database for jobs ready to be executed
func (js *JobScheduler) getJobsToRun(now time.Time) ([]ScheduledJob, error) {
	rows, err := js.server.db.Query(`
		SELECT id, name, source_agent_id, target_agent_id, schedule_type, status,
			   last_scheduled_run, next_scheduled_run
		FROM sync_jobs 
		WHERE schedule_type != 'continuous' 
		AND status = 'active'
		AND next_scheduled_run <= $1
		ORDER BY next_scheduled_run ASC
		LIMIT 50
	`, now)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var jobs []ScheduledJob
	for rows.Next() {
		var job ScheduledJob
		if err := rows.Scan(
			&job.ID, &job.Name, &job.SourceAgentID, &job.TargetAgentID,
			&job.ScheduleType, &job.Status, &job.LastScheduledRun, &job.NextScheduledRun,
		); err != nil {
			log.Printf("❌ Failed to scan job row: %v", err)
			continue
		}
		jobs = append(jobs, job)
	}

	return jobs, nil
}

// executeJob triggers the sync for both source and target agents
func (js *JobScheduler) executeJob(job ScheduledJob) error {
	now := time.Now()
	folderID := fmt.Sprintf("job-%d", job.ID)

	log.Printf("🔄 Executing scheduled job %d (%s): %s -> %s", 
		job.ID, job.Name, job.SourceAgentID, job.TargetAgentID)

	// Trigger scan on source agent (where files originate)
	scanMessage := map[string]interface{}{
		"type":      "scan-folder",
		"folder_id": folderID,
	}

	if err := js.server.sendJobToAgent(job.SourceAgentID, scanMessage); err != nil {
		return fmt.Errorf("failed to trigger scan on source agent %s: %w", job.SourceAgentID, err)
	}

	// Update job execution times
	nextRun := js.calculateNextRun(job.ScheduleType, &now)
	
	_, err := js.server.db.Exec(`
		UPDATE sync_jobs 
		SET last_scheduled_run = $1, next_scheduled_run = $2, updated_at = $3
		WHERE id = $4
	`, now, nextRun, now, job.ID)
	
	if err != nil {
		return fmt.Errorf("failed to update job execution times: %w", err)
	}

	log.Printf("📅 Job %d next run scheduled for %s", job.ID, nextRun.Format("2006-01-02 15:04:05"))
	return nil
}

// calculateNextRun determines the next execution time based on schedule type
func (js *JobScheduler) calculateNextRun(scheduleType string, lastRun *time.Time) time.Time {
	now := time.Now()
	
	switch scheduleType {
	case "hourly":
		if lastRun == nil {
			// First run: next full hour
			return time.Date(now.Year(), now.Month(), now.Day(), now.Hour()+1, 0, 0, 0, now.Location())
		}
		return lastRun.Add(1 * time.Hour)
		
	case "daily":
		if lastRun == nil {
			// First run: tomorrow at midnight
			return time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, now.Location())
		}
		return lastRun.Add(24 * time.Hour)
		
	default:
		// Default to hourly for unknown schedule types
		log.Printf("⚠️ Unknown schedule type: %s, defaulting to hourly", scheduleType)
		return now.Add(1 * time.Hour)
	}
}

// GetScheduledJobsStatus returns status of all scheduled jobs for monitoring
func (js *JobScheduler) GetScheduledJobsStatus() (map[string]interface{}, error) {
	rows, err := js.server.db.Query(`
		SELECT schedule_type, COUNT(*) as count,
			   COUNT(CASE WHEN next_scheduled_run <= NOW() THEN 1 END) as ready_to_run,
			   MIN(next_scheduled_run) as next_run
		FROM sync_jobs 
		WHERE schedule_type != 'continuous' AND status = 'active'
		GROUP BY schedule_type
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	status := map[string]interface{}{
		"scheduler_running": js.running,
		"schedules": make(map[string]interface{}),
	}

	schedules := make(map[string]interface{})
	for rows.Next() {
		var scheduleType string
		var count, readyToRun int
		var nextRun *time.Time

		if err := rows.Scan(&scheduleType, &count, &readyToRun, &nextRun); err != nil {
			continue
		}

		schedules[scheduleType] = map[string]interface{}{
			"total_jobs": count,
			"ready_to_run": readyToRun,
			"next_run": nextRun,
		}
	}

	status["schedules"] = schedules
	return status, nil
}