
my_tool_version = "1.2.3"

block {
    input = "value"

    nested_block "name" {
        other_input = "value"
    }

    nested_block "second" {
    }
}

block {
    // ...
}