-- powerpoint-convert.applescript
-- Convert a PowerPoint presentation to PDF via Microsoft PowerPoint.
--
-- Usage: osascript powerpoint-convert.applescript INPUT OUTPUT

on run argv
    if (count of argv) < 2 then
        error "usage: powerpoint-convert.applescript INPUT OUTPUT"
    end if
    set inputPath to item 1 of argv
    set outputPath to item 2 of argv

    set inputPosix to POSIX file inputPath
    set outputPosix to POSIX file outputPath

    tell application "Microsoft PowerPoint"
        activate
        set thePres to open inputPosix
        try
            save thePres in outputPosix as save as PDF
        on error errMsg number errNum
            close thePres saving no
            error errMsg number errNum
        end try
        close thePres saving no
    end tell
end run
