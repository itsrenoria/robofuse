import os
import json
import shutil
import re

from PTT import Parser, add_defaults
from PTT.handlers import add_defaults
from PTT.anime import anime_handler

# Config
BASE_DIR = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
LIBRARY_DIR = os.path.join(BASE_DIR, "Library")
CONFIG_FILE = os.path.join(BASE_DIR, "config.json")
TRACKING_FILE = os.path.join(BASE_DIR, "cache", "file_tracking.json")
ORGANIZER_DB = os.path.join(BASE_DIR, "cache", "organizer_db.json")

def get_organized_dir():
    """Read organized_dir from config, fallback to ./Organized"""
    if os.path.exists(CONFIG_FILE):
        with open(CONFIG_FILE, 'r') as f:
            config = json.load(f)
            organized = config.get("organized_dir", "./Organized")
            # Resolve relative path from BASE_DIR
            if not os.path.isabs(organized):
                return os.path.join(BASE_DIR, organized.lstrip("./"))
            return organized
    return os.path.join(BASE_DIR, "Organized")

ORGANIZED_DIR = get_organized_dir()

def load_config():
    if os.path.exists(CONFIG_FILE):
        with open(CONFIG_FILE, 'r') as f:
            return json.load(f)
    return {}

def load_tracking():
    if os.path.exists(TRACKING_FILE):
        with open(TRACKING_FILE, 'r') as f:
            return json.load(f)
    return {}

def load_organizer_db():
    if os.path.exists(ORGANIZER_DB):
        with open(ORGANIZER_DB, 'r') as f:
            return json.load(f)
    return {}

def save_organizer_db(db):
    try:
        with open(ORGANIZER_DB, 'w') as f:
            json.dump(db, f, indent=2)
    except Exception as e:
        print(f"Error saving organizer DB: {e}")

def get_rd_id_from_link(link):
    """Extract ID from RD link (e.g. https://real-debrid.com/d/ABCD1234XYZ -> ABCD1234XYZ)"""
    if not link:
        return ""
    match = re.search(r'/d/([a-zA-Z0-9]+)', link)
    if match:
        return match.group(1)
    return ""

def clean_filename(name):
    """Remove special characters but keep spaces, dots, dashes, parentheses"""
    # Replace illegal filesystem chars
    clean = re.sub(r'[<>:"/\\|?*]', '', name)
    clean = re.sub(r'[<>:"/\\|?*]', '', name)
    return clean

def find_existing_series_folder(base_folder, title, year):
    """
    Check if a folder for the series already exists in the destination to prevent duplicates.
    Example: If target is "Mr. Robot" but "Mr. Robot (2015)" exists, return "Mr. Robot (2015)".
    """
    search_dir = os.path.join(ORGANIZED_DIR, base_folder)
    if not os.path.exists(search_dir):
        return None
        
    candidates = []
    try:
        candidates = os.listdir(search_dir)
    except OSError:
        return None
        
    normalized_title = title.lower().strip()
    target_with_year = f"{title} ({year})" if year else title
    
    # Check for exact matches first (case-insensitive)
    for folder in candidates:
        if folder.lower() == target_with_year.lower():
            return folder

    # Check for title match with/without year
    # Case: We have "Mr. Robot", existing is "Mr. Robot (2015)"
    # Case: We have "Mr. Robot (2015)", existing is "Mr. Robot"
    for folder in candidates:
        # Simple heuristic: folder starts with title
        if folder.lower().startswith(normalized_title):
            # Verify it's actually the same show. 
            # If folder is "Mr. Robot (2015)" and title is "Mr. Robot" -> Match
            # If folder is "The Flash (2014)" and title is "The Flash" -> Match
            
            # Check if the remaining part is just year or empty
            remainder = folder.lower().replace(normalized_title, "").strip()
            if not remainder or (remainder.startswith("(") and remainder.endswith(")") and len(remainder) == 6):
                return folder
                
    return None

def get_content_type_and_paths(parsed, parent_parsed, filename, rd_id):
    """
    Determine content type and destination path based on PTT results, potentially falling back to parent folder info.
    Returns: (type, destination_relative_path)
    """
    # Extract info from filename
    f_title = parsed.get("title")
    f_year = parsed.get("year")
    f_season = parsed.get("seasons", [])
    f_episode = parsed.get("episodes", [])
    f_anime = parsed.get("anime", False)

    # Extract info from parent
    p_title = parent_parsed.get("title")
    p_year = parent_parsed.get("year")
    p_season = parent_parsed.get("seasons", [])
    p_episode = parent_parsed.get("episodes", [])
    p_anime = parent_parsed.get("anime", False)

    # Decide on "Series-ness"
    # It is a series if:
    # 1. Filename has Season/Episode info or Anime flag
    # 2. Filename looks like a Movie (no S/E) BUT Parent looks like a Series (S/E or Anime flag)
    
    is_series_filename = bool(f_season or f_episode or f_anime)
    is_series_parent = bool(p_season or p_episode or p_anime)
    
    # Logic:
    # If Parent says it's a series, WE TRUST THE PARENT for the Series Name/Year.
    # This solves "Death by Lightning" (Parsed as "Death by") and "The Expanse" (Parsed as "Interview...").
    
    if is_series_parent:
         final_type = "anime" if p_anime else "series"
         
         # Trust Parent Parsing for Show Identity
         title = p_title if p_title else "Unknown"
         year = p_year if p_year else f_year
         
         # PRIORITY FIX:
         # Does filename have specific season info?
         if f_season:
             season = f_season
         else:
             # Fallback to parent season (e.g. for extras)
             season = p_season
         
         # Does filename have specific episode info?
         if f_episode:
             episode = f_episode
         else:
             # Usually parent doesn't have episode info for a whole series folder,
             # but if it did, we could use it? Unlikely.
             episode = [] 
         
    elif is_series_filename:
        # Standard case: Parent doesn't look like a series (maybe root folder?), but filename does.
        final_type = "anime" if f_anime else "series"
        title = f_title if f_title else "Unknown"
        year = f_year
        season = f_season
        episode = f_episode
        
    else:
        # Both look like movies or unknown.
        final_type = "movie"
        title = f_title if f_title else (p_title if p_title else "Unknown")
        year = f_year if f_year else p_year

    # Determine base folder and structure
    if final_type == "anime":
        base_folder = "Anime"
    elif final_type == "series":
        base_folder = "Series"
    else:
        base_folder = "Movies"

    # -- FOLDER NORMALIZATION / DEDUPLICATION --
    # Before we finalize the title, check if a folder already exists for this Series/Movie.
    # Especially for "Mr. Robot" vs "Mr. Robot (2015)"
    
    existing_folder = find_existing_series_folder(base_folder, title, year)
    if existing_folder:
        # Use the existing folder name exactly
        formatted_title = existing_folder
    else:
        # Construct new
        formatted_title = f"{title}"
        if year:
            formatted_title += f" ({year})"
        formatted_title = clean_filename(formatted_title)

    # ID String
    id_suffix = f" [{rd_id}]" if rd_id else ""

    # Extension (preserve original)
    ext = os.path.splitext(filename)[1]
    
    if final_type == "movie":
        final_filename = clean_filename(f"{formatted_title}{id_suffix}{ext}")
        dest_path = os.path.join("Movies", formatted_title, final_filename)
    
    else: # Series or Anime
        # Determine Season Folder
        if season:
            season_num = season[0]
            season_folder = f"Season {season_num:02d}"
        else:
            season_folder = "Season Unknown"

        # Determine Episode String
        if episode:
            # We have episode info
            if season:
                 ep_str = f"S{season[0]:02d}E{episode[0]:02d}"
            else:
                 ep_str = f"E{episode[0]:02d}"
            
            final_filename = clean_filename(f"{title} {ep_str}{id_suffix}{ext}")

        else:
            # No episode info.
            # This is likely the "Creating the First Season" case.
            # We want: "SeriesName - OriginalName[ID].strm" to correspond to the folder.
            
            part_name = f_title if f_title else "Unknown"
            
            # If Show Name is same as Part Name, don't repeat.
            if part_name.lower() == title.lower():
                 final_filename = clean_filename(f"{title}{id_suffix}{ext}")
            else:
                 final_filename = clean_filename(f"{title} - {part_name}{id_suffix}{ext}")
        
        dest_path = os.path.join(base_folder, formatted_title, season_folder, final_filename)

    return final_type, dest_path

def process_library():
    config = load_config()
    if not config.get("ptt_rename", False):
        return

    # Initialize parser
    parser = Parser()
    add_defaults(parser)
    anime_handler(parser)

    tracking = load_tracking()
    organizer_db = load_organizer_db()
    
    # 1. Identify valid source items (current library state)
    current_source_paths = set()
    
    # Track which destination files correlate to which source file
    # Map: source_relative_path -> { "dest_path": ..., "rd_id": ... }
    new_state = {}

    changes_count = 0
    errors_count = 0
    skipped_count = 0



    for rel_path, meta in tracking.items():
        current_source_paths.add(rel_path)
        
        # Check if we already have this organized and up to date
        if rel_path in organizer_db:
            # Check if source link/ID hasn't changed
            prev_entry = organizer_db[rel_path]
            # Extract current ID
            current_id = get_rd_id_from_link(meta.get("link"))
            if prev_entry.get("rd_id") == current_id and os.path.exists(os.path.join(ORGANIZED_DIR, prev_entry["dest_path"])):
                # Up to date
                new_state[rel_path] = prev_entry
                skipped_count += 1
                continue

        # Needs organization
        source_full_path = os.path.join(LIBRARY_DIR, rel_path)
        
        if not os.path.exists(source_full_path):
            continue

        try:
            # Parse Filename
            filename = os.path.basename(rel_path)
            name_no_ext = os.path.splitext(filename)[0]
            parsed = parser.parse(name_no_ext)
            
            # Parse Parent Folder
            parent_rel_dir = os.path.dirname(rel_path)
            # We want the immediate parent folder name, e.g. "Better Call Saul Season 1"
            # Since rel_path includes the structure inside Library, dirname gives us that path.
            # If rel_path is just "Movie.strm", dirname is empty string.
            parent_folder_name = os.path.basename(parent_rel_dir) if parent_rel_dir else ""
            
            parent_parsed = {}
            if parent_folder_name:
                 parent_parsed = parser.parse(parent_folder_name)

            rd_id = get_rd_id_from_link(meta.get("link"))
            
            # Determine Destination
            content_type, dest_rel_path = get_content_type_and_paths(parsed, parent_parsed, filename, rd_id)
            dest_full_path = os.path.join(ORGANIZED_DIR, dest_rel_path)
            
            # Create dirs
            os.makedirs(os.path.dirname(dest_full_path), exist_ok=True)
            
            # Copy file (using copy2 to preserve metadata)
            # We use copy instead of move because Robofuse manages the Library folder
            shutil.copy2(source_full_path, dest_full_path)
            
            # Update state
            new_state[rel_path] = {
                "dest_path": dest_rel_path,
                "rd_id": rd_id,
                "type": content_type,
                "updated_at": meta.get("last_checked")
            }
            changes_count += 1
            # print(f"Organized: {dest_rel_path}")

        except Exception as e:
            print(f"Error organizing {rel_path}: {e}")
            errors_count += 1

    # 2. Cleanup (Delete organized files that no longer exist in source)
    deleted_count = 0
    
    # Identify files in ORGANIZED_DIR that are no longer in new_state
    # We iterate over the OLD organizer_db
    for old_src_path, old_entry in organizer_db.items():
        if old_src_path not in current_source_paths:
            # Source file was deleted, remove organized file
            dest_rel = old_entry["dest_path"]
            dest_full = os.path.join(ORGANIZED_DIR, dest_rel)
            
            if os.path.exists(dest_full):
                try:
                    os.remove(dest_full)
                    
                    # Try to remove empty parent directories
                    parent = os.path.dirname(dest_full)
                    try:
                        # Recursively remove empty dirs up to Organized root
                        while parent != ORGANIZED_DIR and parent.startswith(ORGANIZED_DIR):
                            if not os.listdir(parent):
                                os.rmdir(parent)
                                parent = os.path.dirname(parent)
                            else:
                                break
                    except OSError:
                        pass # Directory not empty or other error
                        
                    deleted_count += 1
                except Exception as e:
                    print(f"Error deleting {dest_rel}: {e}")

    # 3. Save new state
    save_organizer_db(new_state)

    # Output JSON for Go to parse
    result = {
        "processed": len(tracking),
        "new": changes_count,
        "deleted": deleted_count,
        "updated": 0,  # Currently we don't track updates separately
        "skipped": skipped_count,
        "errors": errors_count
    }
    print(json.dumps(result))

if __name__ == "__main__":
    process_library()
