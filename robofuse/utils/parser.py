"""Filename parsing utilities using PTT (Parsett)."""

from typing import Dict, Any, List, Optional, Tuple
import os

try:
    from PTT import Parser, add_defaults
    from PTT.anime import anime_handler
    PTT_AVAILABLE = True
except ImportError:
    PTT_AVAILABLE = False

from robofuse.utils.logging import logger


# Common anime titles to help with detection
COMMON_ANIME_TITLES = [
    "one piece", "dragon ball", "naruto", "attack on titan", "demon slayer", 
    "my hero academia", "jujutsu kaisen", "bleach", "hunter x hunter", 
    "evangelion", "fullmetal", "gintama", "death note", "sword art online",
    "cowboy bebop", "fairy tail", "jojo", "pokemon", "yu-gi-oh", "sailor moon",
    "boku no hero", "shingeki no kyojin", "kimetsu no yaiba", "berserk", "gundam"
]


class MetadataParser:
    """Parse filenames to extract metadata using PTT."""
    
    def __init__(self, enabled: bool = True):
        """Initialize the parser.
        
        Args:
            enabled: Whether to use PTT for parsing.
        """
        self.enabled = enabled and PTT_AVAILABLE
        
        if self.enabled:
            logger.info("PTT parser enabled for metadata extraction")
            # Create a parser instance
            self.parser = Parser()
            # Add default handlers
            add_defaults(self.parser)
            # Add anime handlers
            anime_handler(self.parser)
        elif not PTT_AVAILABLE:
            logger.warning("PTT library not available. Install with 'pip install git+https://github.com/dreulavelle/PTT.git' for better filename parsing.")
            self.enabled = False
        else:
            logger.info("PTT parser disabled")
    
    def parse(self, filename: str) -> Dict[str, Any]:
        """Parse a filename to extract metadata.
        
        Args:
            filename: The filename to parse
            
        Returns:
            Dictionary with extracted metadata
        """
        if not self.enabled or not hasattr(self, 'parser'):
            logger.warning(f"PTT parser not available for {filename}. File will be placed in Others folder.")
            return {"title": os.path.splitext(filename)[0], "type": "unknown"}
        
        try:
            # Use PTT's built-in parsing
            metadata = self.parser.parse(filename)
            logger.verbose(f"Parsed metadata: {metadata}")
            
            # Ensure there's a valid title
            if not metadata.get("title"):
                metadata["type"] = "unknown"
                return {"title": os.path.splitext(filename)[0], "type": "unknown"}
            
            # Determine content type and formatted title
            media_type, formatted_title = self._determine_media_type(metadata)
            metadata["type"] = media_type.lower()
            metadata["formatted_title"] = formatted_title
            
            return metadata
        except Exception as e:
            logger.warning(f"Error parsing filename with PTT: {str(e)}. File will be placed in Others folder.")
            return {"title": os.path.splitext(filename)[0], "type": "unknown"}
    
    def _determine_media_type(self, metadata: Dict[str, Any]) -> Tuple[str, str]:
        """Determine media type and create formatted title based on PTT parsed data.
        
        Args:
            metadata: The parsed metadata from PTT
            
        Returns:
            Tuple of (media_type, formatted_title)
        """
        title = metadata.get('title', 'Unknown')
        resolution = metadata.get('resolution', '')
        quality = metadata.get('quality', '')
        
        # Create a format suffix if quality information is available
        format_suffix = ""
        if resolution or quality:
            format_parts = []
            if resolution:
                format_parts.append(resolution)
            if quality:
                format_parts.append(quality)
            if format_parts:
                format_suffix = f" [{', '.join(format_parts)}]"
        
        # Check if it's explicitly marked as anime
        if metadata.get('anime', False):
            return self._format_anime_title(metadata, title, format_suffix)
        
        # Check for common anime release groups
        if metadata.get('group', '').lower() in ['subsplease', 'erai-raws', 'horrible', 'anime time', 'horriblesubs']:
            return self._format_anime_title(metadata, title, format_suffix)
        
        # Check for common anime titles
        if any(anime_title in title.lower() for anime_title in COMMON_ANIME_TITLES):
            # If it has episodes but no seasons, it's likely anime
            if metadata.get('episodes') and not metadata.get('seasons'):
                return self._format_anime_title(metadata, title, format_suffix)
        
        # If it has seasons and episodes, format as TV Show
        if metadata.get('seasons') and metadata.get('episodes'):
            # Even with seasons/episodes, it might still be anime if the title matches
            if any(anime_title in title.lower() for anime_title in COMMON_ANIME_TITLES):
                # This is a special case - anime formatted with TV show season/episode
                # We'll keep the TV Show formatting but categorize as anime
                season = metadata['seasons'][0]
                episode = metadata['episodes'][0]
                return "Anime", f"{title} S{season:02d}E{episode:02d}{format_suffix}"
            
            # Regular TV show
            media_type = "TV Show"
            season = metadata['seasons'][0]
            episode = metadata['episodes'][0]
            return media_type, f"{title} S{season:02d}E{episode:02d}{format_suffix}"
        
        # Files with just episodes but no seasons could be anime
        elif metadata.get('episodes') and not metadata.get('seasons'):
            # Default to TV Show with E## format if not identified as anime
            media_type = "TV Show"
            episode = metadata['episodes'][0]
            return media_type, f"{title} E{episode:02d}{format_suffix}"
            
        # Default to movie if no season/episode info
        year_str = f" ({metadata['year']})" if metadata.get('year') else ""
        return "Movie", f"{title}{year_str}{format_suffix}"
    
    def _format_anime_title(self, metadata: Dict[str, Any], title: str, format_suffix: str) -> Tuple[str, str]:
        """Format anime title consistently.
        
        Args:
            metadata: The parsed metadata
            title: The title of the anime
            format_suffix: Format suffix with resolution/quality
            
        Returns:
            Tuple of (media_type, formatted_title)
        """
        media_type = "Anime"
        episodes = metadata.get('episodes', [])
        
        if episodes and metadata.get('seasons'):
            # Anime with seasons and episodes
            season = metadata['seasons'][0]
            episode = episodes[0]
            return media_type, f"{title} S{season:02d}E{episode:02d}{format_suffix}"
        elif episodes:
            # Anime with just episode numbers, no seasons
            episode = episodes[0]
            return media_type, f"{title} - {episode:03d}{format_suffix}"
        
        # Anime with no episode info
        return media_type, f"{title}{format_suffix}"
    
    def generate_folder_structure(self, metadata: Dict[str, Any]) -> List[str]:
        """Generate a folder structure based on metadata.
        
        Args:
            metadata: The parsed metadata
            
        Returns:
            List of folder names for the path
        """
        folder_structure = []
        media_type = metadata.get("type", "unknown").lower()
        
        if media_type == "unknown":
            folder_structure.append("Others")
        elif media_type == "tv show":
            folder_structure.append("TV Shows")
            folder_structure.append(metadata["title"])
            if metadata.get("seasons") and metadata["seasons"]:
                season_num = metadata["seasons"][0]
                folder_structure.append(f"Season {season_num:02d}")
        elif media_type == "movie":
            folder_structure.append("Movies")
            movie_folder = metadata["title"]
            if metadata.get("year"):
                movie_folder += f" ({metadata['year']})"
            folder_structure.append(movie_folder)
        elif media_type == "anime":
            folder_structure.append("Anime")
            folder_structure.append(metadata["title"])
            if metadata.get("seasons") and metadata["seasons"]:
                season_num = metadata["seasons"][0]
                folder_structure.append(f"Season {season_num:02d}")
        else:
            folder_structure.append("Others")
            if metadata.get("title"):
                folder_structure.append(metadata["title"])
        
        return folder_structure
    
    def generate_filename(self, metadata: Dict[str, Any], download_id: Optional[str] = None) -> str:
        """Generate a clean filename based on metadata.
        
        Args:
            metadata: The parsed metadata
            download_id: Optional download ID to append to the filename
            
        Returns:
            A clean filename (without extension)
        """
        filename = metadata.get("formatted_title", metadata.get("title", "Unknown"))
        
        # Append download ID if provided
        if download_id:
            filename = f"{filename} [{download_id}]"
        
        return filename