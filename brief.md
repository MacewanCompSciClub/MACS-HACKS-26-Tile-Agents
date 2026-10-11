| Bit | 7 , 8 , 6 | 5 | 4 | 3 | 2 | 1 | 0 |
|-----|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|
| Use | Health | IsPlayer | IsDecaying | F | V | V | V |


Here is an example game loop:
```python
while(connected == True):
    round_over = False
    world_state = wait_for_round_start()

    while not (round_over == False):
        wait_for_next_update(world_state)
        next_move = calculate_next_move(world_state)
        send_move_to_server(next_move)
```
