provider "google" {
  project = var.project_id
  region  = var.region
}

# ==========================================
# Pub/Sub Topic for Helius Ingestion
# ==========================================
resource "google_pubsub_topic" "helius_ingestion_stream" {
  name = "helius-ingestion-stream"
  
  labels = {
    environment = "production"
    service     = "ingestor"
  }
}

# ==========================================
# Cloud Run Service for Go Ingestor
# ==========================================
resource "google_cloud_run_service" "ingestor_service" {
  name     = "itswork-ingestor"
  location = var.region

  template {
    spec {
      containers {
        image = "gcr.io/${var.project_id}/itswork-ingestor:latest"
        
        env {
          name  = "PORT"
          value = "8080"
        }
        
        # Security Policy: Inject Neon DB config from Secret Manager
        env {
          name = "DATABASE_URL"
          value_from {
            secret_key_ref {
              name = "neon-db-url"
              key  = "latest"
            }
          }
        }
      }
    }
  }

  traffic {
    percent         = 100
    latest_revision = true
  }
}
