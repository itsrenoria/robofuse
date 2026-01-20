import os
import re
from pathlib import Path
from typing import Dict, List, Set, Optional, Any, Tuple

from robofuse.utils.logging import logger
from robofuse.utils.parser import MetadataParser


class StrmFile:
    """Class for handling .strm files."""
    
    def __init__(self, output_dir: str, use_ptt_parser: bool = True):
        self.output_dir = Path(output_dir)
        self._ensure_output_dir()
        self.use_ptt_parser = use_ptt_parser
        self.metadata_parser = MetadataParser(enabled=use_ptt_parser)
    
    def _ensure_output_dir(self):
        """Ensure the output directory exists."""
        if not self.output_dir.exists():
            logger.info(f"Creating output directory: {self.output_dir}")
            self.output_dir.mkdir(parents=True, exist_ok=True)
    
    def _sanitize_filename(self, filename: str) -> str:
        """Sanitize a filename to be safe for the filesystem."""
        # Replace illegal characters
        sanitized = re.sub(r'[<>:"/\\|?*]', "_", filename)
        # Replace multiple spaces with a single space
        sanitized = re.sub(r'\s+', ' ', sanitized)
        # Trim leading/trailing spaces
        sanitized = sanitized.strip()
        # Ensure filename isn't too long
        if len(sanitized) > 240:
            sanitized = sanitized[:240]
        
        return sanitized
    
    def create_or_update_strm(
        self, 
        download_url: str, 
        filename: str, 
        torrent_name: str, 
        dry_run: bool = False,
        download_id: Optional[str] = None
    ) -> Dict[str, Any]:
        """
        Create or update a .strm file.
        
        Args:
            download_url: The URL to include in the .strm file
            filename: The filename for the .strm file (without extension)
            torrent_name: The name of the torrent (used for the folder)
            dry_run: If True, don't actually create/update the file
            download_id: Optional download ID to append to the filename
            
        Returns:
            Dictionary with status and details
        """
        if self.use_ptt_parser:
            # Parse filename to extract metadata
            metadata = self.metadata_parser.parse(filename)
            logger.verbose(f"Metadata for {filename}: {metadata}")
            
            # Generate folder structure based on metadata
            folder_parts = self.metadata_parser.generate_folder_structure(metadata)
            # Create the full path
            folder_path = self.output_dir
            for part in folder_parts:
                folder_path = folder_path / self._sanitize_filename(part)
            
            # Generate filename based on metadata and download ID
            base_filename = self.metadata_parser.generate_filename(metadata, download_id)
            safe_filename = self._sanitize_filename(base_filename)
        else:
            # Fallback to using torrent name as the folder
            folder_path = self.output_dir / self._sanitize_filename(torrent_name)
            
            # Fallback to original filename without adding download_id
            safe_filename = self._sanitize_filename(filename)
        
        # Add .strm extension if missing
        if not safe_filename.lower().endswith('.strm'):
            strm_filename = f"{safe_filename}.strm"
        else:
            strm_filename = safe_filename
        
        # Full path to the .strm file
        strm_path = folder_path / strm_filename
        
        # Check if this is an update or new file
        is_update = strm_path.exists()
        
        # Get current content if file exists
        current_url = None
        if is_update:
            try:
                with open(strm_path, 'r') as f:
                    current_url = f.read().strip()
            except Exception as e:
                logger.warning(f"Failed to read existing STRM file: {str(e)}")
        
        # Determine action to take
        if is_update and current_url == download_url:
            logger.verbose(f"STRM file already exists with current URL: {strm_path}")
            return {
                "status": "skipped",
                "path": str(strm_path),
                "reason": "file exists with same URL",
                "is_update": False
            }
        
        if dry_run:
            action = "Would update" if is_update else "Would create"
            logger.info(f"{action} STRM file: {strm_path}")
            return {
                "status": "dry_run",
                "path": str(strm_path),
                "action": "update" if is_update else "create",
                "is_update": is_update
            }
        
        # Create directory if it doesn't exist
        if not folder_path.exists():
            folder_path.mkdir(parents=True, exist_ok=True)
        
        # Write the .strm file
        try:
            with open(strm_path, 'w') as f:
                f.write(download_url)
            
            action = "Updated" if is_update else "Created"
            logger.success(f"{action} STRM file: {strm_path}")
            
            return {
                "status": "success",
                "path": str(strm_path),
                "action": "update" if is_update else "create",
                "is_update": is_update
            }
        except Exception as e:
            logger.error(f"Failed to write STRM file: {str(e)}")
            return {
                "status": "error",
                "path": str(strm_path),
                "error": str(e)
            }
    
    def delete_strm(self, strm_path: str) -> Dict[str, Any]:
        """Delete a .strm file."""
        path = Path(strm_path)
        
        if not path.exists():
            logger.warning(f"STRM file does not exist: {path}")
            return {
                "status": "error",
                "path": str(path),
                "error": "File does not exist"
            }
        
        try:
            path.unlink()
            logger.success(f"Deleted STRM file: {path}")
            
            # Remove empty parent directory if it's now empty
            parent = path.parent
            if parent.exists() and not any(parent.iterdir()):
                parent.rmdir()
                logger.info(f"Removed empty directory: {parent}")
            
            return {
                "status": "success",
                "path": str(path)
            }
        except Exception as e:
            logger.error(f"Failed to delete STRM file: {str(e)}")
            return {
                "status": "error",
                "path": str(path),
                "error": str(e)
            }
    
    def find_existing_strm_files(self) -> List[Dict[str, Any]]:
        """Find all existing .strm files in the output directory."""
        logger.info(f"Scanning for existing STRM files in {self.output_dir}")
        
        strm_files = []
        
        # Walk through the output directory
        for root, _, files in os.walk(self.output_dir):
            for file in files:
                if file.lower().endswith('.strm'):
                    strm_path = os.path.join(root, file)
                    
                    # Read the URL from the STRM file
                    try:
                        with open(strm_path, 'r') as f:
                            url = f.read().strip()
                        
                        # Extract relative path from output_dir
                        rel_path = os.path.relpath(strm_path, self.output_dir)
                        
                        # Get parts of the path for organized content
                        path_parts = Path(rel_path).parts
                        
                        file_info = {
                            "path": strm_path,
                            "url": url,
                            "filename": os.path.basename(strm_path)
                        }
                        
                        # Add path parts info (useful for organized content)
                        if len(path_parts) >= 2:
                            file_info["parent_folder"] = path_parts[0]
                            if len(path_parts) >= 3 and "season" in path_parts[1].lower():
                                file_info["season_folder"] = path_parts[1]
                        
                        # Add metadata from the filename
                        if self.use_ptt_parser:
                            try:
                                file_metadata = self.metadata_parser.parse(os.path.basename(strm_path))
                                file_info["metadata"] = file_metadata
                            except Exception as e:
                                logger.debug(f"Failed to parse metadata for {strm_path}: {str(e)}")
                        
                        strm_files.append(file_info)
                    except Exception as e:
                        logger.warning(f"Failed to read STRM file {strm_path}: {str(e)}")
        
        logger.info(f"Found {len(strm_files)} existing STRM files")
        return strm_files 