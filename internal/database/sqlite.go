package database

import (
	"context"
	"database/sql"
	"os"

	_ "github.com/mattn/go-sqlite3"
	"github.com/pardnchiu/go-podrun/internal/model"
)

type SQLite struct {
	db *sql.DB
}

type ContainerRecord struct {
	LocalDir  string `json:"local_dir"`
	RemoteDir string `json:"remote_dir"`
	File      string `json:"file"`
	Hostname  string `json:"hostname"`
	IP        string `json:"ip"`
	Content   string `json:"content"`
}

func NewSQLite(dbPath string) (*SQLite, error) {
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, err
	}

	s := &SQLite{db: db}
	if err := s.create(); err != nil {
		db.Close()
		return nil, err
	}

	return s, nil
}

func (s *SQLite) create() error {
	schema, err := os.ReadFile("sql/create.sql")
	if err != nil {
		return err
	}

	_, err = s.db.Exec(string(schema))
	return err
}

func (s *SQLite) UpsertPod(ctx context.Context, d *model.Pod) error {
	_, err := s.db.ExecContext(ctx, `
  INSERT INTO pods (
    uid, pod_uid, pod_name, local_dir, remote_dir,
    file, target, status, hostname, ip,
    replicas
  )
  VALUES (
    ?, ?, ?, ?, ?,
    ?, ?, ?, ?, ?,
    ?
  )
  ON CONFLICT(uid) DO UPDATE SET
    pod_name = excluded.pod_name,
    local_dir = excluded.local_dir,
    remote_dir = excluded.remote_dir,
    file = excluded.file,
    target = excluded.target,
    status = excluded.status,
    hostname = excluded.hostname,
    ip = excluded.ip,
    replicas = excluded.replicas,
    updated_at = CURRENT_TIMESTAMP,
    dismiss = 0
  `,
		d.UID,
		d.PodID,
		d.PodName,
		d.LocalDir,
		d.RemoteDir,
		d.File,
		d.Target,
		d.Status,
		d.Hostname,
		d.IP,
		d.Replicas,
	)
	return err
}

func (s *SQLite) UpdatePod(ctx context.Context, d *model.Pod) error {
	_, err := s.db.ExecContext(ctx, `
  UPDATE pods
  SET
    status = ?,
    updated_at = CURRENT_TIMESTAMP,
    dismiss = ?
  WHERE uid = ?
  `,
		d.Status,
		d.Dismiss,
		d.UID,
	)
	return err
}

func (s *SQLite) InsertRecord(ctx context.Context, d *model.Record) error {
	_, err := s.db.ExecContext(ctx, `
  INSERT INTO records (
    pod_id, content, hostname, ip
  )
  VALUES (
    (SELECT id FROM pods WHERE uid = ?), ?, ?, ?
  )
  `,
		d.UID,
		d.Content,
		d.Hostname,
		d.IP,
	)
	return err
}

func (s *SQLite) ListPods(ctx context.Context) ([]model.Pod, error) {
	rows, err := s.db.QueryContext(ctx, `
	SELECT
	  id, uid, pod_uid, pod_name, local_dir,
		remote_dir, file, target, status, hostname,
		ip, replicas, created_at, updated_at
	FROM pods
	WHERE dismiss = 0
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var containers []model.Pod
	for rows.Next() {
		var c model.Pod
		if err := rows.Scan(
			&c.ID, &c.UID, &c.PodID, &c.PodName, &c.LocalDir,
			&c.RemoteDir, &c.File, &c.Target, &c.Status, &c.Hostname,
			&c.IP, &c.Replicas, &c.CreatedAt, &c.UpdatedAt,
		); err != nil {
			return nil, err
		}
		containers = append(containers, c)
	}

	return containers, rows.Err()
}

func (s *SQLite) ContainerInfo(ctx context.Context, uid string) (*model.Pod, error) {
	row := s.db.QueryRowContext(ctx, `
  SELECT
    id, uid, pod_uid, pod_name, local_dir,
    remote_dir, file, target, status, hostname,
    ip, replicas, created_at, updated_at
  FROM pods
  WHERE dismiss = 0 AND uid = ?
  LIMIT 1
  `, uid)

	var c model.Pod
	err := row.Scan(
		&c.ID, &c.UID, &c.PodID, &c.PodName, &c.LocalDir,
		&c.RemoteDir, &c.File, &c.Target, &c.Status, &c.Hostname,
		&c.IP, &c.Replicas, &c.CreatedAt, &c.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}

	return &c, nil
}

func (s *SQLite) ListContainerRecords(ctx context.Context, uid string) ([]ContainerRecord, error) {
	rows, err := s.db.QueryContext(ctx, `
  SELECT
    pods.local_dir,
    pods.remote_dir,
    pods.file,
    pods.hostname,
    pods.ip,
    records.content
  FROM records
  LEFT JOIN pods ON records.pod_id = pods.id
  WHERE pods.dismiss = 0 AND pods.uid = ?
  `, uid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var records []ContainerRecord
	for rows.Next() {
		var r ContainerRecord
		if err := rows.Scan(&r.LocalDir, &r.RemoteDir, &r.File,
			&r.Hostname, &r.IP, &r.Content); err != nil {
			return nil, err
		}
		records = append(records, r)
	}

	return records, rows.Err()
}

func (s *SQLite) Close() error {
	return s.db.Close()
}
