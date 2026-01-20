import json
import os
from pathlib import Path
from typing import Dict, Any


DEFAULT_CONFIG = {
    "token": "YOUR_RD_API_TOKEN",
    "output_dir": "./Library",
    "cache_dir": "./cache", 
    "concurrent_requests": 32,
    "general_rate_limit": 60,
    "torrents_rate_limit": 25,
    "watch_mode_interval": 60,
    "repair_torrents": True,
    "use_ptt_parser": True,
}


class Config:
    def __init__(self, config_path: str = None):
        self.config_path = config_path or "config.json"
        self.config = self._load_config()
        self._validate_config()
        self._setup_directories()

    def _load_config(self) -> Dict[str, Any]:
        """Load configuration from file or create default if not found."""
        config = DEFAULT_CONFIG.copy()
        
        if os.path.exists(self.config_path):
            try:
                with open(self.config_path, "r") as f:
                    user_config = json.load(f)
                    config.update(user_config)
            except json.JSONDecodeError:
                print(f"Error parsing config file {self.config_path}. Using defaults.")
            except Exception as e:
                print(f"Error loading config file: {e}. Using defaults.")
        else:
            print(f"Config file {self.config_path} not found. Using defaults.")
            self._save_default_config()
            
        return config
    
    def _save_default_config(self):
        """Save default configuration to file."""
        try:
            with open(self.config_path, "w") as f:
                json.dump(DEFAULT_CONFIG, f, indent=4)
            print(f"Default configuration saved to {self.config_path}")
        except Exception as e:
            print(f"Error saving default config: {e}")
    
    def _validate_config(self):
        """Validate configuration values."""
        if self.config["token"] == DEFAULT_CONFIG["token"]:
            print("WARNING: You are using the default API token. Please update your config.json with your Real-Debrid API token.")
        
        # Convert paths to absolute
        self.config["output_dir"] = os.path.abspath(os.path.expanduser(self.config["output_dir"]))
        self.config["cache_dir"] = os.path.abspath(os.path.expanduser(self.config["cache_dir"]))
        
        # Ensure numeric values are reasonable
        self.config["concurrent_requests"] = max(1, min(int(self.config["concurrent_requests"]), 64))
        self.config["general_rate_limit"] = max(1, int(self.config["general_rate_limit"]))
        self.config["torrents_rate_limit"] = max(1, int(self.config["torrents_rate_limit"]))
        self.config["watch_mode_interval"] = max(30, int(self.config["watch_mode_interval"]))
        
    def _setup_directories(self):
        """Create output and cache directories if they don't exist."""
        for dir_name in ["output_dir", "cache_dir"]:
            directory = Path(self.config[dir_name])
            if not directory.exists():
                try:
                    directory.mkdir(parents=True)
                    print(f"Created directory: {directory}")
                except Exception as e:
                    print(f"Error creating directory {directory}: {e}")
    
    def get(self, key: str, default: Any = None) -> Any:
        """Get configuration value by key."""
        return self.config.get(key, default)
    
    def override(self, overrides: Dict[str, Any]):
        """Override configuration with provided values."""
        self.config.update(overrides)
        self._validate_config()
        
    def __getitem__(self, key: str) -> Any:
        """Allow dictionary-like access to config values."""
        return self.config[key] 