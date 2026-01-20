from typing import Dict, List, Any, Optional

from robofuse.api.client import RealDebridClient
from robofuse.utils.logging import logger


class DownloadsAPI:
    """API client for Real-Debrid downloads endpoints."""
    
    def __init__(self, client: RealDebridClient):
        self.client = client
    
    def get_downloads(self, page: int = 1, limit: int = 100) -> List[Dict[str, Any]]:
        """Get a list of downloads from Real-Debrid."""
        logger.info(f"Fetching downloads (page {page}, limit {limit})")
        return self.client.get("downloads", params={"page": page, "limit": limit})
    
    def get_all_downloads(self) -> List[Dict[str, Any]]:
        """Get all downloads using pagination."""
        logger.info("Fetching all downloads (this may take a while)")
        return self.client.get_paginated("downloads", limit_per_page=100)
    
    def get_download_info(self, download_id: str) -> Dict[str, Any]:
        """Get information about a specific download."""
        logger.verbose(f"Fetching info for download {download_id}")
        return self.client.get(f"downloads/info/{download_id}")
    
    def delete_download(self, download_id: str) -> Dict[str, Any]:
        """Delete a download from Real-Debrid."""
        logger.info(f"Deleting download {download_id}")
        return self.client.delete(f"downloads/delete/{download_id}")
    
    def filter_streamable_downloads(self, downloads: List[Dict[str, Any]]) -> List[Dict[str, Any]]:
        """Filter downloads to only include streamable ones."""
        streamable_downloads = [
            download for download in downloads
            if download.get("streamable") == 1
        ]
        
        logger.info(f"Filtered {len(streamable_downloads)} streamable downloads out of {len(downloads)} total")
        return streamable_downloads
    
    def filter_unique_downloads(self, downloads: List[Dict[str, Any]]) -> List[Dict[str, Any]]:
        """Filter downloads to remove duplicates based on link."""
        # Group downloads by link
        downloads_by_link = {}
        
        for download in downloads:
            link = download.get("link")
            if not link:
                continue
                
            # If we already have this link, only keep the newer one
            if link in downloads_by_link:
                existing_generated = downloads_by_link[link].get("generated", "")
                current_generated = download.get("generated", "")
                
                if current_generated > existing_generated:
                    downloads_by_link[link] = download
            else:
                downloads_by_link[link] = download
        
        unique_downloads = list(downloads_by_link.values())
        
        logger.info(f"Filtered {len(unique_downloads)} unique downloads out of {len(downloads)} total")
        return unique_downloads 