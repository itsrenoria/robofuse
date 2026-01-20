import concurrent.futures
from typing import List, Callable, TypeVar, Any, Dict, Optional
from tqdm import tqdm

from robofuse.utils.logging import logger

T = TypeVar('T')
R = TypeVar('R')


def parallel_process(
    items: List[T],
    process_func: Callable[[T], R],
    max_workers: int = 32,
    desc: str = "Processing",
    show_progress: bool = True
) -> List[R]:
    """
    Process a list of items in parallel using a ThreadPoolExecutor.
    
    Args:
        items: List of items to process
        process_func: Function to apply to each item
        max_workers: Maximum number of worker threads
        desc: Description for the progress bar
        show_progress: Whether to show a progress bar
        
    Returns:
        List of results
    """
    if not items:
        logger.info(f"No items to process for {desc}")
        return []
    
    results = []
    n_items = len(items)
    
    # Adjust max_workers if we have fewer items than workers
    actual_workers = min(max_workers, n_items)
    
    logger.info(f"Processing {n_items} items with {actual_workers} workers ({desc})")
    
    with concurrent.futures.ThreadPoolExecutor(max_workers=actual_workers) as executor:
        # Create a dictionary mapping futures to their indices
        future_to_index = {
            executor.submit(process_func, item): i
            for i, item in enumerate(items)
        }
        
        # Create progress bar if requested
        if show_progress:
            futures_iter = tqdm(
                concurrent.futures.as_completed(future_to_index),
                total=len(items),
                desc=desc
            )
        else:
            futures_iter = concurrent.futures.as_completed(future_to_index)
        
        # Process results as they complete
        for future in futures_iter:
            try:
                result = future.result()
                results.append(result)
            except Exception as e:
                logger.error(f"Error processing item: {str(e)}")
                # Append None for failed tasks to maintain order
                results.append(None)
    
    # Sort results according to their original order
    sorted_results = [None] * n_items
    for future, index in future_to_index.items():
        try:
            sorted_results[index] = future.result()
        except Exception:
            # We already logged the error above
            pass
    
    # Filter out None results if any
    filtered_results = [r for r in sorted_results if r is not None]
    
    if len(filtered_results) != n_items:
        logger.warning(f"Failed to process {n_items - len(filtered_results)} items")
    
    return filtered_results 