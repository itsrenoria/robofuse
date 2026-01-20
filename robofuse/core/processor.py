import os
import time
from datetime import datetime, timedelta
from typing import Dict, List, Any, Set, Optional, Tuple
import json

from robofuse.api.client import RealDebridClient
from robofuse.api.torrents import TorrentsAPI
from robofuse.api.downloads import DownloadsAPI
from robofuse.api.unrestrict import UnrestrictAPI
from robofuse.core.strm import StrmFile
from robofuse.utils.logging import logger
from robofuse.utils.concurrency import parallel_process
from robofuse.config import Config


class RoboFuseProcessor:
    """Main processor for robofuse service."""
    
    def __init__(self, config: Config, dry_run: bool = False):
        self.config = config
        self.dry_run = dry_run
        
        # Initialize API client
        self.client = RealDebridClient(
            token=config["token"],
            general_rate_limit=config["general_rate_limit"],
            torrents_rate_limit=config["torrents_rate_limit"]
        )
        
        # Initialize API modules
        self.torrents_api = TorrentsAPI(self.client)
        self.downloads_api = DownloadsAPI(self.client)
        self.unrestrict_api = UnrestrictAPI(self.client)
        
        # Initialize STRM handler with PTT parser option
        self.strm_handler = StrmFile(
            config["output_dir"],
            use_ptt_parser=config.get("use_ptt_parser", True)
        )
        
        # Cache file for state
        self.cache_dir = config["cache_dir"]
        os.makedirs(self.cache_dir, exist_ok=True)
        self.cache_file = os.path.join(self.cache_dir, "state_cache.json")
    
    def _load_cache(self) -> Dict[str, Any]:
        """Load cached state if available."""
        if os.path.exists(self.cache_file):
            try:
                with open(self.cache_file, 'r') as f:
                    return json.load(f)
            except Exception as e:
                logger.warning(f"Failed to load cache: {str(e)}")
        
        return {
            "last_run": None,
            "torrent_link_map": {},
            "processed_torrents": [],
            "processed_downloads": []
        }
    
    def _save_cache(self, cache_data: Dict[str, Any]):
        """Save state to cache."""
        try:
            with open(self.cache_file, 'w') as f:
                json.dump(cache_data, f, indent=2)
            logger.verbose(f"Saved cache to {self.cache_file}")
        except Exception as e:
            logger.warning(f"Failed to save cache: {str(e)}")
    
    def run(self):
        """Run the main processing pipeline."""
        if self.dry_run:
            logger.info("Running in DRY RUN mode - no files will be created or modified")
        
        # Start timing
        start_time = time.time()
        
        # Load cache
        cache = self._load_cache()
        
        # Step 1: Get and filter torrents
        torrents = self._get_and_filter_torrents()
        
        # Step 2: If repair_torrents is enabled, reinsert dead torrents
        if self.config["repair_torrents"]:
            self._reinsert_dead_torrents(torrents["dead"])
        
        # Step 3: Get and filter downloads
        downloads = self._get_and_filter_downloads()
        
        # Step 4: Find torrent links without downloads
        pending_links, link_to_torrent = self._find_pending_links(torrents["active"], downloads["filtered"])
        
        # Step 5: Unrestrict pending links
        if pending_links:
            logger.info(f"Found {len(pending_links)} links without corresponding downloads")
            unrestricted = self._unrestrict_links(pending_links)
            if unrestricted:
                # Refresh downloads list with new unrestricted links
                downloads = self._get_and_filter_downloads()
        else:
            logger.info("All torrent links already have corresponding downloads")
        
        # Step 6: Generate release candidates
        candidates = self._generate_release_candidates(torrents["active"], downloads["filtered"], link_to_torrent)
        
        # Step 7: Process STRM files (create/update/delete)
        self._process_strm_files(candidates)
        
        # Update cache with latest run info
        cache["last_run"] = datetime.now().isoformat()
        self._save_cache(cache)
        
        # Display summary
        elapsed_time = time.time() - start_time
        logger.info(f"Processing completed in {elapsed_time:.2f} seconds")
        
        return {
            "torrents_processed": len(torrents["active"]),
            "downloads_processed": len(downloads["filtered"]),
            "pending_links": len(pending_links),
            "candidates": len(candidates),
            "elapsed_time": elapsed_time
        }
    
    def _get_and_filter_torrents(self) -> Dict[str, List[Dict[str, Any]]]:
        """Get and filter torrents."""
        logger.info("Retrieving torrents from Real-Debrid")
        
        # Get all torrents
        all_torrents = self.torrents_api.get_all_torrents()
        logger.info(f"Retrieved {len(all_torrents)} torrents")
        
        # Filter out dead torrents
        dead_torrents = [t for t in all_torrents if t.get("status") == "dead"]
        if dead_torrents:
            logger.warning(f"Found {len(dead_torrents)} dead torrents")
        
        # Filter for downloaded torrents
        active_torrents = [t for t in all_torrents if t.get("status") == "downloaded"]
        logger.info(f"Filtered {len(active_torrents)} active (downloaded) torrents")
        
        return {
            "all": all_torrents,
            "active": active_torrents,
            "dead": dead_torrents
        }
    
    def _reinsert_dead_torrents(self, dead_torrents: List[Dict[str, Any]]):
        """Reinsert dead torrents."""
        if not dead_torrents:
            logger.info("No dead torrents to reinsert")
            return
        
        logger.info(f"Reinserting {len(dead_torrents)} dead torrents")
        
        results = []
        for torrent in dead_torrents:
            torrent_hash = torrent.get("hash")
            if not torrent_hash:
                logger.warning(f"Torrent {torrent.get('id')} has no hash, skipping")
                continue
            
            logger.info(f"Reinserting torrent: {torrent.get('filename', 'Unknown')}")
            
            try:
                # Reinsert the torrent
                result = self.torrents_api.reinsert_torrent(torrent_hash)
                
                if result.get("status") == "success":
                    # Delete the original dead torrent
                    try:
                        self.torrents_api.delete_torrent(torrent["id"])
                        logger.success(f"Deleted original dead torrent {torrent['id']}")
                    except Exception as e:
                        logger.error(f"Failed to delete original dead torrent: {str(e)}")
                
                results.append({
                    "torrent_id": torrent.get("id"),
                    "hash": torrent_hash,
                    "result": result
                })
            except Exception as e:
                logger.error(f"Failed to reinsert torrent {torrent.get('id')}: {str(e)}")
        
        logger.info(f"Reinserted {sum(1 for r in results if r['result'].get('status') == 'success')} torrents")
    
    def _get_and_filter_downloads(self) -> Dict[str, List[Dict[str, Any]]]:
        """Get and filter downloads."""
        logger.info("Retrieving downloads from Real-Debrid")
        
        # Get all downloads
        all_downloads = self.downloads_api.get_all_downloads()
        logger.info(f"Retrieved {len(all_downloads)} downloads")
        
        # Filter for streamable downloads
        streamable_downloads = self.downloads_api.filter_streamable_downloads(all_downloads)
        
        # Filter for unique downloads
        unique_downloads = self.downloads_api.filter_unique_downloads(streamable_downloads)
        
        return {
            "all": all_downloads,
            "streamable": streamable_downloads,
            "filtered": unique_downloads
        }
    
    def _find_pending_links(
        self, 
        torrents: List[Dict[str, Any]], 
        downloads: List[Dict[str, Any]]
    ) -> Tuple[List[str], Dict[str, Dict[str, Any]]]:
        """Find torrent links without corresponding downloads."""
        # Create a set of download links for faster lookup
        download_links = {d.get("link", "") for d in downloads if d.get("link")}
        
        # Build a mapping from link to torrent for easier reference later
        link_to_torrent = {}
        
        # Find links without corresponding downloads
        pending_links = []
        
        for torrent in torrents:
            links = torrent.get("links", [])
            torrent_id = torrent.get("id", "")
            
            for link in links:
                # Add to the mapping
                link_to_torrent[link] = torrent
                
                # Check if this link has a download
                if link not in download_links:
                    pending_links.append(link)
        
        return pending_links, link_to_torrent
    
    def _unrestrict_links(self, links: List[str]) -> List[Dict[str, Any]]:
        """Unrestrict pending links."""
        if self.dry_run:
            logger.info(f"[DRY RUN] Would unrestrict {len(links)} links")
            return []
        
        logger.info(f"Unrestricting {len(links)} links")
        
        # Define single link processor function for parallel processing
        def unrestrict_single_link(link):
            try:
                return self.unrestrict_api.unrestrict_link(link)
            except Exception as e:
                logger.error(f"Failed to unrestrict link: {str(e)}")
                return None
        
        # Process in parallel
        results = parallel_process(
            links,
            unrestrict_single_link,
            max_workers=self.config["concurrent_requests"],
            desc="Unrestricting links",
            show_progress=True
        )
        
        # Filter out None results (failed unrestrictions)
        successful_results = [r for r in results if r is not None]
        
        logger.info(f"Successfully unrestricted {len(successful_results)} out of {len(links)} links")
        return successful_results
    
    def _generate_release_candidates(
        self, 
        torrents: List[Dict[str, Any]], 
        downloads: List[Dict[str, Any]],
        link_to_torrent: Dict[str, Dict[str, Any]]
    ) -> List[Dict[str, Any]]:
        """Generate release candidates for STRM files."""
        logger.info("Generating STRM file candidates")
        
        candidates = []
        
        for download in downloads:
            link = download.get("link", "")
            if not link:
                continue
                
            # Get the corresponding torrent
            torrent = link_to_torrent.get(link)
            if not torrent:
                logger.warning(f"No torrent found for link: {link}")
                continue
            
            download_url = download.get("download", "")
            if not download_url:
                logger.warning(f"No download URL found for download: {download.get('id', '')}")
                continue
            
            candidates.append({
                "download_url": download_url,
                "filename": download.get("filename", ""),
                "torrent_name": torrent.get("filename", ""),
                "download_id": download.get("id", ""),
                "torrent_id": torrent.get("id", ""),
                "download": download,
                "torrent": torrent
            })
        
        logger.info(f"Generated {len(candidates)} STRM file candidates")
        return candidates
    
    def _process_strm_files(self, candidates: List[Dict[str, Any]]):
        """Process STRM files for the given candidates."""
        logger.info(f"Processing {len(candidates)} STRM files")
        
        for candidate in candidates:
            try:
                # Create or update STRM file
                result = self.strm_handler.create_or_update_strm(
                    download_url=candidate["download_url"],
                    filename=candidate["filename"],
                    torrent_name=candidate["torrent_name"],
                    dry_run=self.dry_run,
                    download_id=candidate.get("download_id")
                )
                
                if result["status"] == "error":
                    logger.error(f"Failed to process STRM file: {result['error']}")
                elif result["status"] == "dry_run":
                    logger.info(f"[DRY RUN] {result['action']} STRM file: {result['path']}")
                elif result["status"] == "success":
                    logger.success(f"{result['action']} STRM file: {result['path']}")
                
            except Exception as e:
                logger.error(f"Error processing STRM file for {candidate['filename']}: {str(e)}")
    
    def watch(self, interval: Optional[int] = None):
        """Run the service in watch mode."""
        watch_interval = interval or self.config["watch_mode_interval"]
        
        logger.info(f"Starting watch mode (interval: {watch_interval} seconds)")
        
        try:
            while True:
                logger.info(f"Running processing cycle at {datetime.now().isoformat()}")
                self.run()
                
                logger.info(f"Sleeping for {watch_interval} seconds until next cycle")
                time.sleep(watch_interval)
        except KeyboardInterrupt:
            logger.info("Watch mode interrupted by user")
        except Exception as e:
            logger.error(f"Error in watch mode: {str(e)}")
            raise 