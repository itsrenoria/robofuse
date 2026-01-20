from typing import Dict, List, Any, Optional

from robofuse.api.client import RealDebridClient, APIError
from robofuse.utils.logging import logger


class UnrestrictAPI:
    """API client for Real-Debrid unrestrict endpoints."""
    
    def __init__(self, client: RealDebridClient):
        self.client = client
    
    def unrestrict_link(self, link: str, password: Optional[str] = None, remote: int = 0) -> Dict[str, Any]:
        """Unrestrict a link to get download info."""
        logger.info(f"Unrestricting link")
        logger.verbose(f"Link: {link}")
        
        data = {"link": link}
        
        if password:
            data["password"] = password
        
        if remote == 1:
            data["remote"] = 1
        
        try:
            result = self.client.post("unrestrict/link", data=data)
            logger.success(f"Successfully unrestricted link")
            return result
        except APIError as e:
            logger.error(f"Failed to unrestrict link: {e.message}")
            raise
    
    def check_link(self, link: str) -> Dict[str, Any]:
        """Check if a link is supported by Real-Debrid."""
        logger.verbose(f"Checking link: {link}")
        return self.client.post("unrestrict/check", data={"link": link})
    
    def batch_unrestrict_links(self, links: List[str], max_retries: int = 3) -> List[Dict[str, Any]]:
        """Unrestrict multiple links with retries."""
        logger.info(f"Batch unrestricting {len(links)} links")
        
        results = []
        failed_links = []
        
        for link in links:
            try:
                result = self.unrestrict_link(link)
                results.append(result)
            except Exception as e:
                logger.warning(f"Failed to unrestrict link on first attempt: {str(e)}")
                failed_links.append(link)
        
        # Retry failed links
        if failed_links:
            logger.info(f"Retrying {len(failed_links)} failed links")
            
            retry_count = 0
            while failed_links and retry_count < max_retries:
                retry_count += 1
                logger.info(f"Retry attempt {retry_count}/{max_retries}")
                
                still_failed = []
                for link in failed_links:
                    try:
                        result = self.unrestrict_link(link)
                        results.append(result)
                        logger.success(f"Successfully unrestricted link on retry {retry_count}")
                    except Exception as e:
                        logger.warning(f"Failed on retry {retry_count}: {str(e)}")
                        still_failed.append(link)
                
                failed_links = still_failed
        
        # Final report
        if failed_links:
            logger.warning(f"Failed to unrestrict {len(failed_links)} links after {max_retries} retries")
        
        logger.info(f"Successfully unrestricted {len(results)} links")
        return results 