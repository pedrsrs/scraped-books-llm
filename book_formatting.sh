#!/bin/bash

edit_headers() {
    file=$1
    initial_pattern='^.**** START OF THE PROJECT GUTENBERG EBOOK.*$'
    final_pattern='^.**** END OF THE PROJECT GUTENBERG EBOOK.*$'

    header_line=$(grep -n "$initial_pattern" "$file" | cut -d: -f1)
    footer_line=$(grep -n "$final_pattern" "$file" | cut -d: -f1)

    temp_file=$(mktemp)

    sed "1,${header_line}d; ${footer_line},\$d" "$file" > "$temp_file"
    mv "$temp_file" "$file"
}

edit_non_latin_characters() {
    file=$1

    temp_file=$(mktemp)

    grep -oP '\([^\)]+\)|\[[^\]]+\]|[\p{Latin}\p{P}\p{Z}0-9]+' "$file" > "$temp_file"
    mv "$temp_file" "$file"
}

concatenate_books() {
    concatenated_file="complete_books.txt"
    > "$concatenated_file"  

    for processed_file in ./books/*; do
        if [ -f "$processed_file" ]; then
            cat "$processed_file" >> "$concatenated_file"
            echo -e "\n" >> "$concatenated_file"  
        fi
    done

    echo "All books have been concatenated into $concatenated_file"
}

main() {
    books_dir="./books"

    if [ ! -d "$books_dir" ]; then
        echo "Books directory not found!"
        exit 1
    fi

    for book_file in "$books_dir"/*; do
        if [ -f "$book_file" ]; then
            edit_headers "$book_file"
            edit_non_latin_characters "$book_file"
            echo "Edited $book_file"
        fi
    done

    concatenate_books
}

main
