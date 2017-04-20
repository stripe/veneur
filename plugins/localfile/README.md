LocalFile Plugin
==================

The LocalFile Plugin appends each flush as TSV data to a specified file on the local system.  Since the file path is not parametrized with regards to date or time, the file with the TSV data should be rotated, processed, or removed to avoid problems with filling the disk.

You can enable the LocalFile plugin by setting the `flush_file` key in the configuration to a file path.  The path must be writeable by Veneur, and if the file does not exist, Veneur will try to create it.
