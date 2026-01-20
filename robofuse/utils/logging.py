"""Logging configuration for robofuse."""

import logging
import sys
from enum import Enum
from typing import Optional

import colorama
from colorama import Fore, Style


# Initialize colorama
colorama.init()

# Define custom log levels
VERBOSE = 15
logging.addLevelName(VERBOSE, "VERBOSE")


class LogLevel(Enum):
    ERROR = 1
    WARNING = 2
    INFO = 3
    VERBOSE = 4
    DEBUG = 5


class CustomLogger(logging.Logger):
    """Custom logger with additional verbosity levels."""
    
    def __init__(self, name: str, level: LogLevel = LogLevel.INFO):
        super().__init__(name)
        self.level = level
        self._setup_handlers()
    
    def _setup_handlers(self):
        """Setup console handler with colored output."""
        console_handler = logging.StreamHandler(sys.stdout)
        console_handler.setFormatter(logging.Formatter('%(message)s'))
        self.addHandler(console_handler)
    
    def set_level(self, level: LogLevel):
        """Set the logging level."""
        self.level = level
        if level == LogLevel.DEBUG:
            super().setLevel(logging.DEBUG)
        elif level == LogLevel.VERBOSE:
            super().setLevel(VERBOSE)
        elif level == LogLevel.INFO:
            super().setLevel(logging.INFO)
        elif level == LogLevel.WARNING:
            super().setLevel(logging.WARNING)
        elif level == LogLevel.ERROR:
            super().setLevel(logging.ERROR)
    
    def verbose(self, msg, *args, **kwargs):
        """Log a verbose message."""
        if self.isEnabledFor(VERBOSE):
            self._log(VERBOSE, f"{Fore.CYAN}[VERBOSE] {msg}{Style.RESET_ALL}", args, **kwargs)
    
    def debug(self, msg, *args, **kwargs):
        """Log a debug message."""
        if self.isEnabledFor(logging.DEBUG):
            self._log(logging.DEBUG, f"{Fore.MAGENTA}[DEBUG] {msg}{Style.RESET_ALL}", args, **kwargs)
    
    def info(self, msg, *args, **kwargs):
        """Log an info message."""
        if self.isEnabledFor(logging.INFO):
            self._log(logging.INFO, f"{Fore.GREEN}[INFO] {msg}{Style.RESET_ALL}", args, **kwargs)
    
    def warning(self, msg, *args, **kwargs):
        """Log a warning message."""
        if self.isEnabledFor(logging.WARNING):
            self._log(logging.WARNING, f"{Fore.YELLOW}[WARNING] {msg}{Style.RESET_ALL}", args, **kwargs)
    
    def error(self, msg, *args, **kwargs):
        """Log an error message."""
        if self.isEnabledFor(logging.ERROR):
            self._log(logging.ERROR, f"{Fore.RED}[ERROR] {msg}{Style.RESET_ALL}", args, **kwargs)
    
    def success(self, msg, *args, **kwargs):
        """Log a success message."""
        if self.isEnabledFor(logging.INFO):
            self._log(logging.INFO, f"{Fore.GREEN}[SUCCESS] {msg}{Style.RESET_ALL}", args, **kwargs)
    
    def progress(self, msg, *args, **kwargs):
        """Log a progress message without newline."""
        if self.isEnabledFor(logging.INFO):
            print(f"{Fore.BLUE}[PROGRESS] {msg}{Style.RESET_ALL}", end="\r", flush=True)


# Global logger instance
logger = CustomLogger("robofuse")

def setup_logging(verbosity: Optional[str] = None) -> None:
    """Setup logging configuration.
    
    Args:
        verbosity: Logging level ('debug', 'verbose', 'info', 'warning', 'error')
    """
    if verbosity:
        verbosity = verbosity.lower()
        if verbosity == 'debug':
            logger.set_level(LogLevel.DEBUG)
        elif verbosity == 'verbose':
            logger.set_level(LogLevel.VERBOSE)
        elif verbosity == 'info':
            logger.set_level(LogLevel.INFO)
        elif verbosity == 'warning':
            logger.set_level(LogLevel.WARNING)
        elif verbosity == 'error':
            logger.set_level(LogLevel.ERROR)
    else:
        logger.set_level(LogLevel.INFO)  # Default to INFO level 