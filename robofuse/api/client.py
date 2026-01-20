import time
from typing import Optional, Dict, Any, List, Union, Tuple
import json
from urllib.parse import urljoin

import requests
from requests.exceptions import RequestException

from robofuse.utils.logging import logger


class APIError(Exception):
    """Exception raised for API errors."""
    def __init__(self, message: str, code: Optional[int] = None, response: Optional[requests.Response] = None):
        self.message = message
        self.code = code
        self.response = response
        super().__init__(message)


class RealDebridClient:
    """Client for the Real-Debrid API."""
    
    BASE_URL = "https://api.real-debrid.com/rest/1.0/"
    
    def __init__(self, token: str, general_rate_limit: int = 60, torrents_rate_limit: int = 25):
        self.token = token
        self.general_rate_limit = general_rate_limit
        self.torrents_rate_limit = torrents_rate_limit
        
        # Track requests for rate limiting
        self.last_general_request_time = 0
        self.last_torrents_request_time = 0
        
        # Set up session
        self.session = requests.Session()
        self.session.headers.update({
            "Authorization": f"Bearer {token}",
            "User-Agent": "robofuse/0.3.0",
            "Content-Type": "application/json",
        })
    
    def _rate_limit(self, endpoint: str):
        """Apply rate limiting based on the endpoint."""
        current_time = time.time()
        
        # Torrents API has stricter rate limiting
        if "/torrents" in endpoint:
            time_since_last = current_time - self.last_torrents_request_time
            wait_time = (1 / self.torrents_rate_limit) - time_since_last
            
            if wait_time > 0:
                logger.debug(f"Rate limiting for torrents API: sleeping for {wait_time:.2f}s")
                time.sleep(wait_time)
            
            self.last_torrents_request_time = time.time()
        else:
            time_since_last = current_time - self.last_general_request_time
            wait_time = (1 / self.general_rate_limit) - time_since_last
            
            if wait_time > 0:
                logger.debug(f"Rate limiting for general API: sleeping for {wait_time:.2f}s")
                time.sleep(wait_time)
            
            self.last_general_request_time = time.time()
    
    def _handle_response(self, response: requests.Response) -> Dict[str, Any]:
        """Handle the API response and raise appropriate exceptions."""
        try:
            # Real-Debrid can return empty response for some successful calls
            if not response.text:
                return {}
            
            data = response.json()
            
            if response.status_code >= 400:
                error_code = data.get("error_code", 0)
                error_message = data.get("error", f"API Error: {response.status_code}")
                logger.error(f"API Error ({error_code}): {error_message}")
                raise APIError(error_message, error_code, response)
            
            return data
        except json.JSONDecodeError:
            # Handle case where response isn't JSON
            if response.status_code >= 400:
                logger.error(f"API Error ({response.status_code}): {response.text}")
                raise APIError(f"API Error: {response.status_code}", response.status_code, response)
            return {"text": response.text}
    
    def request(
        self, 
        method: str, 
        endpoint: str, 
        params: Optional[Dict[str, Any]] = None, 
        data: Optional[Union[Dict[str, Any], str]] = None,
        files: Optional[Dict[str, Any]] = None,
        retry_count: int = 3
    ) -> Dict[str, Any]:
        """Make a request to the API with retries and rate limiting."""
        url = urljoin(self.BASE_URL, endpoint)
        logger.debug(f"Making {method} request to {url}")
        
        attempts = 0
        while attempts < retry_count:
            try:
                self._rate_limit(endpoint)
                
                response = self.session.request(
                    method=method,
                    url=url,
                    params=params,
                    data=data,
                    files=files,
                    timeout=30,  # 30 second timeout
                )
                
                return self._handle_response(response)
            
            except (RequestException, APIError) as e:
                attempts += 1
                retry_wait = min(2 ** attempts, 60)  # Exponential backoff, max 60s
                
                # Certain errors should not be retried
                if isinstance(e, APIError) and e.code in [400, 401, 403, 404]:
                    raise
                
                if attempts < retry_count:
                    logger.warning(f"Request failed: {str(e)}. Retrying in {retry_wait}s (attempt {attempts}/{retry_count})")
                    time.sleep(retry_wait)
                else:
                    logger.error(f"Request failed after {retry_count} attempts: {str(e)}")
                    raise
        
        # This should never happen but just in case
        raise APIError("Maximum retries exceeded")
    
    def get(self, endpoint: str, params: Optional[Dict[str, Any]] = None) -> Dict[str, Any]:
        """Make a GET request to the API."""
        return self.request("GET", endpoint, params=params)
    
    def post(
        self, 
        endpoint: str, 
        params: Optional[Dict[str, Any]] = None, 
        data: Optional[Union[Dict[str, Any], str]] = None,
        files: Optional[Dict[str, Any]] = None
    ) -> Dict[str, Any]:
        """Make a POST request to the API."""
        return self.request("POST", endpoint, params=params, data=data, files=files)
    
    def delete(self, endpoint: str) -> Dict[str, Any]:
        """Make a DELETE request to the API."""
        return self.request("DELETE", endpoint)
    
    def put(
        self, 
        endpoint: str, 
        params: Optional[Dict[str, Any]] = None, 
        data: Optional[Dict[str, Any]] = None
    ) -> Dict[str, Any]:
        """Make a PUT request to the API."""
        return self.request("PUT", endpoint, params=params, data=data)
    
    def get_paginated(
        self, 
        endpoint: str, 
        params: Optional[Dict[str, Any]] = None,
        limit_per_page: int = 100,
        max_pages: Optional[int] = None
    ) -> List[Dict[str, Any]]:
        """Get all pages of results for a paginated endpoint."""
        if params is None:
            params = {}
        
        all_results = []
        page = 1
        
        while True:
            # Copy params to avoid modifying the original
            page_params = params.copy()
            page_params.update({
                "page": page,
                "limit": limit_per_page
            })
            
            logger.verbose(f"Fetching page {page} from {endpoint}")
            results = self.get(endpoint, params=page_params)
            
            # If we get an empty list or dict, we've reached the end
            if not results or (isinstance(results, list) and len(results) == 0):
                break
            
            # Add results to our collection
            if isinstance(results, list):
                all_results.extend(results)
                
                # If we got fewer results than requested, we've reached the end
                if len(results) < limit_per_page:
                    break
            else:
                # Handle case where the API doesn't return a list
                all_results.append(results)
                break
            
            # Check if we've reached the maximum number of pages
            if max_pages and page >= max_pages:
                break
            
            page += 1
        
        return all_results 