output "pubsub_topic_name" {
  description = "The name of the Pub/Sub topic for Helius ingestion stream"
  value       = google_pubsub_topic.helius_ingestion_stream.name
}

output "cloud_run_url" {
  description = "The URL of the deployed Cloud Run Go Ingestor service"
  value       = google_cloud_run_service.ingestor_service.status[0].url
}
