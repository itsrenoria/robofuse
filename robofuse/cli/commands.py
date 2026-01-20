"""Command-line interface for robofuse."""

import click
import os
import sys
from typing import Optional

from robofuse.config import Config
from robofuse.core.processor import RoboFuseProcessor
from robofuse.utils.logging import logger, LogLevel, setup_logging


class Context:
    """Context object to hold shared state."""
    def __init__(self):
        self.config = None
        self.verbosity = "info"


pass_context = click.make_pass_decorator(Context, ensure=True)


@click.group()
@click.version_option(version="0.3.0")
@click.option(
    "--config", "-c",
    type=click.Path(exists=False),
    default="config.json",
    help="Path to config file"
)
@click.option(
    "--verbosity", "-v",
    type=click.Choice(["error", "warning", "info", "verbose", "debug"], case_sensitive=False),
    default="info",
    help="Verbosity level"
)
@click.option("--debug", is_flag=True, help="Enable debug logging")
@click.option("--verbose", is_flag=True, help="Enable verbose logging")
@click.option("--info", is_flag=True, help="Enable info logging") 
@click.option("--warning", is_flag=True, help="Enable warning logging")
@click.option("--error", is_flag=True, help="Enable error logging")
@pass_context
def cli(ctx, config, verbosity, debug, verbose, info, warning, error):
    """robofuse: A service for interacting with Real-Debrid and generating .strm files."""
    
    # Determine verbosity level from flags
    if debug:
        verbosity = "debug"
    elif verbose:
        verbosity = "verbose"
    elif info:
        verbosity = "info"
    elif warning:
        verbosity = "warning"
    elif error:
        verbosity = "error"
    
    # Set up logging
    setup_logging(verbosity)
    
    # Initialize config
    try:
        ctx.config = Config(config_path=config)
        ctx.verbosity = verbosity
    except Exception as e:
        logger.error(f"Failed to initialize configuration: {str(e)}")
        sys.exit(1)


@cli.command()
@pass_context
def run(ctx):
    """Run robofuse once."""
    config = ctx.config
    
    # Check if token is set
    if config["token"] == "YOUR_RD_API_TOKEN":
        logger.error("API token not set. Please update your config.json with your Real-Debrid API token.")
        sys.exit(1)
    
    # Run the processor once
    try:
        processor = RoboFuseProcessor(config)
        processor.run()
    except Exception as e:
        logger.error(f"Error running robofuse: {str(e)}")
        sys.exit(1)


@cli.command()
@click.option(
    "--interval", "-i",
    type=int,
    default=None,
    help="Interval in seconds between processing cycles (defaults to config value)"
)
@pass_context
def watch(ctx, interval):
    """Run robofuse in watch mode."""
    config = ctx.config
    
    # Check if token is set
    if config["token"] == "YOUR_RD_API_TOKEN":
        logger.error("API token not set. Please update your config.json with your Real-Debrid API token.")
        sys.exit(1)
    
    # Run the processor in watch mode
    try:
        processor = RoboFuseProcessor(config)
        processor.watch(interval=interval)
    except Exception as e:
        logger.error(f"Error in watch mode: {str(e)}")
        sys.exit(1)


@cli.command()
@pass_context
def dry_run(ctx):
    """Run robofuse in dry-run mode (no changes made)."""
    config = ctx.config
    
    # Check if token is set
    if config["token"] == "YOUR_RD_API_TOKEN":
        logger.error("API token not set. Please update your config.json with your Real-Debrid API token.")
        sys.exit(1)
    
    # Run the processor in dry-run mode
    try:
        processor = RoboFuseProcessor(config, dry_run=True)
        processor.run()
    except Exception as e:
        logger.error(f"Error in dry-run mode: {str(e)}")
        sys.exit(1) 