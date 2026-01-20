from typing import Dict, List, Any, Optional, Union
import os

from robofuse.api.client import RealDebridClient
from robofuse.utils.logging import logger


class TorrentsAPI:
    """API client for Real-Debrid torrents endpoints."""
    
    def __init__(self, client: RealDebridClient):
        self.client = client
    
    def get_torrents(self, page: int = 1, limit: int = 100) -> List[Dict[str, Any]]:
        """Get a list of torrents from Real-Debrid."""
        logger.info(f"Fetching torrents (page {page}, limit {limit})")
        return self.client.get("torrents", params={"page": page, "limit": limit})
    
    def get_all_torrents(self) -> List[Dict[str, Any]]:
        """Get all torrents using pagination."""
        logger.info("Fetching all torrents (this may take a while)")
        return self.client.get_paginated("torrents", limit_per_page=100)
    
    def get_torrent_info(self, torrent_id: str) -> Dict[str, Any]:
        """Get information about a specific torrent."""
        logger.verbose(f"Fetching info for torrent {torrent_id}")
        return self.client.get(f"torrents/info/{torrent_id}")
    
    def add_magnet(self, magnet_link: str) -> Dict[str, Any]:
        """Add a magnet link to Real-Debrid."""
        logger.info(f"Adding magnet link")
        logger.verbose(f"Magnet: {magnet_link}")
        return self.client.post("torrents/addMagnet", data={"magnet": magnet_link})
    
    def add_torrent_file(self, file_path: str) -> Dict[str, Any]:
        """Upload a torrent file to Real-Debrid."""
        logger.info(f"Uploading torrent file: {os.path.basename(file_path)}")
        
        with open(file_path, "rb") as f:
            files = {"file": (os.path.basename(file_path), f, "application/x-bittorrent")}
            return self.client.post("torrents/addTorrent", files=files)
    
    def select_files(self, torrent_id: str, file_ids: Union[List[int], str] = "all") -> Dict[str, Any]:
        """Select which files to download from the torrent."""
        logger.info(f"Selecting files for torrent {torrent_id}")
        
        if file_ids == "all":
            data = {"files": "all"}
        else:
            # Convert list of IDs to comma-separated string
            files_str = ",".join(str(file_id) for file_id in file_ids)
            data = {"files": files_str}
        
        logger.verbose(f"Selected files: {data['files']}")
        return self.client.post(f"torrents/selectFiles/{torrent_id}", data=data)
    
    def delete_torrent(self, torrent_id: str) -> Dict[str, Any]:
        """Delete a torrent from Real-Debrid."""
        logger.info(f"Deleting torrent {torrent_id}")
        return self.client.delete(f"torrents/delete/{torrent_id}")
    
    def get_torrent_files(self, torrent_id: str) -> List[Dict[str, Any]]:
        """Get a list of files in a torrent."""
        logger.verbose(f"Fetching files for torrent {torrent_id}")
        torrent_info = self.get_torrent_info(torrent_id)
        return torrent_info.get("files", [])
    
    def select_video_files(self, torrent_id: str) -> Dict[str, Any]:
        """Select only video files from a torrent (mp4 and mkv)."""
        logger.info(f"Selecting video files for torrent {torrent_id}")
        
        # Get torrent files
        files = self.get_torrent_files(torrent_id)
        
        # Filter for video files
        video_file_ids = []
        for i, file in enumerate(files):
            file_name = file.get("path", "").lower()
            if file_name.endswith((".mkv", ".mp4")):
                video_file_ids.append(i + 1)  # API uses 1-indexed file IDs
        
        if not video_file_ids:
            logger.warning(f"No video files found in torrent {torrent_id}")
            return {"status": "error", "message": "No video files found"}
        
        logger.verbose(f"Selected {len(video_file_ids)} video files")
        return self.select_files(torrent_id, video_file_ids)
    
    def reinsert_torrent(self, hash_value: str) -> Dict[str, Any]:
        """Reinsert a torrent using its hash."""
        logger.info(f"Reinserting torrent with hash: {hash_value}")
        
        # Create magnet link from hash
        magnet_link = f"magnet:?xt=urn:btih:{hash_value}"
        
        # Add magnet
        add_result = self.add_magnet(magnet_link)
        
        if "id" not in add_result:
            logger.error(f"Failed to add magnet: {add_result}")
            return {"status": "error", "message": "Failed to add magnet", "details": add_result}
        
        torrent_id = add_result["id"]
        logger.success(f"Successfully added magnet. Torrent ID: {torrent_id}")
        
        # Select video files
        select_result = self.select_video_files(torrent_id)
        
        return {
            "status": "success",
            "torrent_id": torrent_id,
            "add_result": add_result,
            "select_result": select_result
        } 