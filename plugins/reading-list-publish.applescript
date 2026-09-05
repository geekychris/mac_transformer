-- reading-list-publish.applescript
-- Add a URL (or file://) to Safari's Reading List. iCloud syncs it to
-- iPhone/iPad automatically.
--
-- Usage: osascript reading-list-publish.applescript PATH_OR_URL
--
-- Safari accepts local file paths as `file://…` URLs in Reading List;
-- iCloud won't sync those to iOS (Reading List is web-first), but they
-- do show up in the Reading List sidebar on this Mac.

on run argv
    if (count of argv) < 1 then
        error "usage: reading-list-publish.applescript PATH_OR_URL"
    end if
    set target to item 1 of argv
    if target does not start with "http" and target does not start with "file:" then
        -- Assume a POSIX path; wrap it.
        set target to "file://" & my percentEncode(target)
    end if

    tell application "Safari"
        add reading list item target
    end tell
end run

on percentEncode(s)
    -- Encode spaces and other unsafe chars via a shell helper so we don't
    -- have to hand-roll URL escaping in AppleScript.
    set encoded to do shell script "python3 -c 'import urllib.parse,sys;print(urllib.parse.quote(sys.argv[1]))' " & quoted form of s
    return encoded
end percentEncode
