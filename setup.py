from setuptools import setup, find_packages

setup(
    name="robofuse",
    version="0.3.0",
    packages=find_packages(),
    install_requires=[
        "requests",
        "click",
        "python-dateutil",
        "tqdm",
        "colorama",
        "parsett",
    ],
    entry_points={
        "console_scripts": [
            "robofuse=robofuse.__main__:main",
        ],
    },
    author="robofuse Team",
    description="A service that interacts with Real-Debrid API to generate .strm files",
    keywords="real-debrid, strm, torrent",
    python_requires=">=3.7",
) 