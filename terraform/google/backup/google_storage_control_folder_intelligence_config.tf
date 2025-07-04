resource "google_storage_control_folder_intelligence_config" "example" {
  name           = "pike-412922"
  edition_config = "STANDARD"
  filter {
    included_cloud_storage_buckets {
      bucket_id_regexes = ["test-id-1*", "test-id-2*"]
    }
    excluded_cloud_storage_locations {
      locations = ["test-location-1*", "test-locations-2*"]
    }
  }
}
